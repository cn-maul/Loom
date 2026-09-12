# release.ps1 - Release pipeline: vet, test, frontend build, backend build, migration smoke.
# Usage: powershell -ExecutionPolicy Bypass -File scripts\release.ps1
# Any step failure exits non-zero. Keep this file ASCII-only: PowerShell 5.1
# reads non-BOM scripts as ANSI and non-ASCII comments can break parsing.
#
# HTTP note: smoke checks use Invoke-WebRequest with proxy disabled. Do NOT
# switch back to curl.exe: under PowerShell 5.1 + redirected output +
# $ErrorActionPreference=Stop, native-command stderr becomes a terminating
# NativeCommandError and invocation itself is unreliable (observed silently
# skipped).

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
$root = $PSScriptRoot | Split-Path
Set-Location $root
[System.Net.WebRequest]::DefaultWebProxy = $null

function Step([string]$name) { Write-Output "`n=== $name ===" }

Add-Type -AssemblyName System.Net.Http | Out-Null
$handler = New-Object System.Net.Http.HttpClientHandler
$handler.UseProxy = $false
$script:client = New-Object System.Net.Http.HttpClient($handler)
$script:client.Timeout = [TimeSpan]::FromSeconds(10)

function Http([string]$method, [string]$path, [string]$json) {
    $uri = "http://127.0.0.1:$script:port$path"
    $req = New-Object System.Net.Http.HttpRequestMessage((New-Object System.Net.Http.HttpMethod($method)), $uri)
    if ($json) {
        $req.Content = New-Object System.Net.Http.ByteArrayContent(, [Text.Encoding]::UTF8.GetBytes($json))
        $req.Content.Headers.ContentType = [System.Net.Http.Headers.MediaTypeHeaderValue]::Parse("application/json")
    }
    $resp = $script:client.SendAsync($req).GetAwaiter().GetResult()
    $code = [int]$resp.StatusCode
    $body = $resp.Content.ReadAsStringAsync().GetAwaiter().GetResult()
    # HttpRequestHeaders.Contains is case-insensitive.
    $hasVersion = $resp.Headers.Contains("X-Api-Version")
    $req.Dispose()
    $resp.Dispose()
    return @{ code = $code; body = $body; hasVersion = $hasVersion }
}

# 1. Static checks
Step "1/6 go vet"
go vet ./...
if ($LASTEXITCODE -ne 0) { Write-Output "go vet failed"; exit 1 }

# 2. Full test suite (contract golden enforced here)
Step "2/6 go test"
go test ./...
if ($LASTEXITCODE -ne 0) { Write-Output "go test failed"; exit 1 }

# 3. Frontend build
Step "3/6 frontend build"
Push-Location "$root\web"
if (-not (Test-Path node_modules)) { npm install }
npm run build
if ($LASTEXITCODE -ne 0) { Pop-Location; Write-Output "frontend build failed"; exit 1 }
Pop-Location

# 4. Embed frontend and build the binary
Step "4/6 go build"
$distSrc = "$root\web\dist"
$distDst = "$root\internal\frontend\dist"
if (Test-Path $distDst) { Remove-Item -Recurse -Force $distDst }
Copy-Item -Recurse $distSrc $distDst
go build -o relationship.exe ./cmd/server
if ($LASTEXITCODE -ne 0) { Write-Output "go build failed"; exit 1 }
if (-not (Test-Path "$root\relationship.exe")) { Write-Output "binary missing"; exit 1 }

# 5. Migration + smoke: temp database, dedicated port, real binary.
Step "5/6 migration + smoke"
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("loom-release-" + [guid]::NewGuid().ToString("N").Substring(0, 8))
New-Item -ItemType Directory -Path $tmp | Out-Null
$script:port = 8177
$script:smokeFailed = $false
@"
server:
  host: 127.0.0.1
  port: $script:port
database:
  path: smoke.db
llm:
  endpoint: http://127.0.0.1:1
  async_extract: false
  allow_remote: false
backup:
  enabled: false
maintenance:
  audit_retention_days: 0
"@ | Set-Content -Encoding UTF8 "$tmp\config.yaml"

$proc = Start-Process -FilePath "$root\relationship.exe" -ArgumentList "config.yaml" `
    -WorkingDirectory $tmp -PassThru -WindowStyle Hidden `
    -RedirectStandardOutput "$tmp\out.log" -RedirectStandardError "$tmp\err.log"
try {
    $ready = $false
    for ($i = 0; $i -lt 60; $i++) {
        Start-Sleep -Milliseconds 500
        if ($proc.HasExited) { break }
        $r = Http "GET" "/api/config" ""
        if ($r.code -eq 200) { $ready = $true; break }
    }
    if (-not $ready) {
        Write-Output "server did not become ready; logs:"
        Get-Content "$tmp\out.log", "$tmp\err.log" -ErrorAction SilentlyContinue | Write-Output
        $script:smokeFailed = $true
        exit 1
    }

    # Contract: every /api response carries the API version header.
    $r = Http "GET" "/api/config" ""
    if (-not $r.hasVersion) {
        $dump = "code=$($r.code) hasVersion=$($r.hasVersion)`nbody: $($r.body)"
        $dump | Set-Content "$tmp\fail_dump.txt" -Encoding UTF8
        Write-Output "missing X-API-Version header (dump: $tmp\fail_dump.txt)"
        $script:smokeFailed = $true
        exit 1
    }

    # Contract: unknown resources answer in the error envelope. Use a missing
    # resource id, not an unknown route: unmatched /api paths fall through to
    # the SPA catch-all by design.
    $r = Http "GET" "/api/persons/does-not-exist" ""
    if ($r.code -ne 404 -or $r.body -notmatch '"ok":false') {
        Write-Output "404 envelope broken: code=$($r.code) body=$($r.body)"
        $script:smokeFailed = $true
        exit 1
    }

    # Maintenance endpoint (phase 6).
    $r = Http "POST" "/api/maintenance/cleanup" ""
    if ($r.code -ne 200 -or $r.body -notmatch '"ok":true') {
        Write-Output "maintenance cleanup failed: code=$($r.code) body=$($r.body)"
        $script:smokeFailed = $true
        exit 1
    }

    Write-Output "smoke passed (version header / 404 envelope / maintenance cleanup)"
}
finally {
    if (-not $proc.HasExited) { Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue }
    Start-Sleep -Milliseconds 300
    if ($script:smokeFailed) { Write-Output "SMOKE FAILED - kept for inspection: $tmp" }
    else { Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue }
}

Step "6/6 result"
Write-Output "RELEASE PIPELINE PASSED: relationship.exe"
exit 0
