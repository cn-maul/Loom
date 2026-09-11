#!/usr/bin/env pwsh
# build.ps1 - 编译项目

$ErrorActionPreference = "Stop"

Write-Host "=== Building Frontend ===" -ForegroundColor Cyan
Push-Location "$PSScriptRoot\web"
npm run build
Pop-Location

Write-Host "=== Copying frontend dist ===" -ForegroundColor Cyan
$distSrc = "$PSScriptRoot\web\dist"
$distDst = "$PSScriptRoot\internal\frontend\dist"
if (Test-Path $distDst) { Remove-Item -Recurse -Force $distDst }
Copy-Item -Recurse $distSrc $distDst

Write-Host "=== Building Go binary ===" -ForegroundColor Cyan
Push-Location $PSScriptRoot
go build -o relationship.exe ./cmd/server
Pop-Location

if (Test-Path "$PSScriptRoot\relationship.exe") {
    Write-Host "Build succeeded: relationship.exe" -ForegroundColor Green
} else {
    Write-Host "Build failed!" -ForegroundColor Red
    exit 1
}
