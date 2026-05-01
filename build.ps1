param(
    [string] $OutputDir = "dist"
)

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

$dist = Join-Path $PSScriptRoot $OutputDir
New-Item -ItemType Directory -Force -Path $dist | Out-Null

$exe = Join-Path $dist "rayinmailresend.exe"
go build -trimpath -ldflags="-s -w" -o $exe .

Write-Host "Built $exe"
