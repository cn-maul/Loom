#!/usr/bin/env pwsh
# run.ps1 - 编译并运行

$ErrorActionPreference = "Stop"
$root = $PSScriptRoot

# 先编译
& "$root\build.ps1"
if ($LASTEXITCODE -ne 0) { exit 1 }

Write-Host "=== Starting server on http://localhost:8080 ===" -ForegroundColor Cyan
& "$root\relationship.exe"
