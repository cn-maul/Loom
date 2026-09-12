# drill.ps1 - Monthly backup & restore drill (fully automated, touches no real data).
# Chain: seed data -> snapshot -> validate -> delete -> staged restore -> restart -> verify.
# Usage: powershell -ExecutionPolicy Bypass -File scripts\drill.ps1
# Pass criterion: last line DRILL PASSED. Keep this file ASCII-only (see release.ps1).
# Uses System.Net.Http.HttpClient, not curl.exe (see release.ps1 header for why).

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
$root = $PSScriptRoot | Split-Path
$exe = "$root\relationship.exe"
if (-not (Test-Path $exe)) { Write-Output "run scripts/release.ps1 first to build relationship.exe"; exit 1 }
[System.Net.WebRequest]::DefaultWebProxy = $null

Add-Type -AssemblyName System.Net.Http | Out-Null
$handler = New-Object System.Net.Http.HttpClientHandler
$handler.UseProxy = $false
$script:client = New-Object System.Net.Http.HttpClient($handler)
$script:client.Timeout = [TimeSpan]::FromSeconds(10)

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("loom-drill-" + [guid]::NewGuid().ToString("N").Substring(0, 8))
New-Item -ItemType Directory -Path "$tmp\backups" | Out-Null
$script:port = 8179
$script:drillFailed = $false
@"
server:
  host: 127.0.0.1
  port: $script:port
database:
  path: drill.db
llm:
  endpoint: http://127.0.0.1:1
  async_extract: false
  allow_remote: false
backup:
  enabled: false
  dir: backups
maintenance:
  audit_retention_days: 0
"@ | Set-Content -Encoding UTF8 "$tmp\config.yaml"

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
    $req.Dispose()
    $resp.Dispose()
    return @{ code = $code; body = $body }
}

function Start-Server {
    $p = Start-Process -FilePath $exe -ArgumentList "config.yaml" `
        -WorkingDirectory $tmp -PassThru -WindowStyle Hidden `
        -RedirectStandardOutput "$tmp\out.log" -RedirectStandardError "$tmp\err.log"
    for ($i = 0; $i -lt 60; $i++) {
        Start-Sleep -Milliseconds 500
        if ($p.HasExited) { break }
        $r = Http "GET" "/api/config" ""
        if ($r.code -eq 200) { return $p }
    }
    Write-Output "server did not start:"
    Get-Content "$tmp\out.log", "$tmp\err.log" -ErrorAction SilentlyContinue | Write-Output
    $script:drillFailed = $true
    exit 1
}

function Require([int]$wantCode, [hashtable]$r, [string]$what) {
    if ($r.code -ne $wantCode) {
        Write-Output "DRILL FAILED at $what : expected $wantCode got $($r.code) body=$($r.body)"
        $script:drillFailed = $true
        exit 1
    }
}

$proc = Start-Server
try {
    Write-Output "[1/6] seed data"
    $r = Http "POST" "/api/persons" '{"name":"drill-person"}'
    Require 201 $r "create person"
    $personID = ($r.body | ConvertFrom-Json).data.id
    $r = Http "POST" "/api/events" ('{"person_id":"' + $personID + '","raw_text":"drill record: backup restore verification","event_date":"2026-09-12"}')
    Require 201 $r "create event"
    $eventID = ($r.body | ConvertFrom-Json).data.event.id
    Write-Output "  person=$personID event=$eventID"

    Write-Output "[2/6] snapshot"
    $r = Http "POST" "/api/backups" ""
    Require 201 $r "backup"
    $file = ($r.body | ConvertFrom-Json).data.backup.name
    Write-Output "  snapshot=$file"

    Write-Output "[3/6] validate snapshot"
    $r = Http "POST" "/api/backups/validate" ('{"file":"' + $file + '"}')
    Require 200 $r "validate"

    Write-Output "[4/6] delete data (disaster simulation)"
    # Delete the event record itself: deleting a person SET NULLs the anchor
    # on purpose (shared records survive), so the event would survive that.
    $r = Http "DELETE" "/api/events/$eventID" ""
    Require 200 $r "delete event"
    $r = Http "GET" "/api/events/$eventID" ""
    Require 404 $r "event after delete"
    Write-Output "  event deleted"

    Write-Output "[5/6] stage restore and restart"
    $r = Http "POST" "/api/backups/restore" ('{"file":"' + $file + '"}')
    Require 202 $r "stage restore"
    Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
    Start-Sleep -Milliseconds 800
    $proc = Start-Server

    Write-Output "[6/6] verify recovery"
    $r = Http "GET" "/api/persons/$personID" ""
    Require 200 $r "person recovered"
    $r = Http "GET" "/api/events/$eventID" ""
    Require 200 $r "event recovered"
    Write-Output "  person and event recovered"
}
finally {
    if (-not $proc.HasExited) { Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue }
    Start-Sleep -Milliseconds 300
    if ($script:drillFailed) { Write-Output "kept for inspection: $tmp" }
    else { Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue }
}
if ($script:drillFailed) { exit 1 }
Write-Output "DRILL PASSED - backup/restore chain fully operational"
