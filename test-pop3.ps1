param(
    [string] $Username = "sunday@rayin.com.tw",
    [string] $Pop3Host = "mail.rayin.com.tw",
    [int] $Pop3Port = 110,
    [string] $Pop3TlsMode = "starttls"
)

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

if ([string]::IsNullOrWhiteSpace($Username)) {
    $Username = Read-Host -Prompt "POP3 username"
}

$securePassword = Read-Host -Prompt "POP3 mailbox password / app password" -AsSecureString
$passwordPtr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($securePassword)

try {
    $plainPassword = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($passwordPtr)
}
finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($passwordPtr)
}

$env:POP3_HOST = $Pop3Host.Trim()
$env:POP3_PORT = "$Pop3Port"
$env:POP3_TLS_MODE = $Pop3TlsMode.Trim()
$env:POP3_USERNAME = $Username.Trim()
$env:POP3_PASSWORD = ($plainPassword -replace '\s', '')

try {
    & (Join-Path $PSScriptRoot "e.ps1") --once --check-login-only
    exit $LASTEXITCODE
}
finally {
    Remove-Item Env:\POP3_HOST -ErrorAction SilentlyContinue
    Remove-Item Env:\POP3_PORT -ErrorAction SilentlyContinue
    Remove-Item Env:\POP3_TLS_MODE -ErrorAction SilentlyContinue
    Remove-Item Env:\POP3_USERNAME -ErrorAction SilentlyContinue
    Remove-Item Env:\POP3_PASSWORD -ErrorAction SilentlyContinue
}
