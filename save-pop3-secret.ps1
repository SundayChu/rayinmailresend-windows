param(
    [string] $Username = "sunday@rayin.com.tw"
)

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

$secretDir = Join-Path $PSScriptRoot "secrets"
$secretFile = Join-Path $secretDir "pop3_password.txt"

New-Item -ItemType Directory -Force -Path $secretDir | Out-Null

$securePassword = Read-Host -Prompt "POP3 password for $Username" -AsSecureString
$securePassword | ConvertFrom-SecureString | Set-Content -LiteralPath $secretFile -Encoding ASCII

Write-Host "Saved encrypted POP3 password for current Windows user:"
Write-Host $secretFile
Write-Host "This file can only be decrypted by the same Windows user on this computer."
