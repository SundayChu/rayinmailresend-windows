param(
    [string] $Username = "sunday@rayin.com.tw"
)

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

$secretDir = Join-Path $PSScriptRoot "secrets"
$secretFile = Join-Path $secretDir "pop3_password.machine"

function Import-ProtectedData {
    try {
        [void][System.Security.Cryptography.ProtectedData]
    }
    catch {
        try {
            Add-Type -AssemblyName System.Security.Cryptography.ProtectedData
        }
        catch {
            Add-Type -AssemblyName System.Security
        }
    }
}

New-Item -ItemType Directory -Force -Path $secretDir | Out-Null
Import-ProtectedData

$securePassword = Read-Host -Prompt "POP3 password for $Username" -AsSecureString
$passwordPtr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($securePassword)

try {
    $plainPassword = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($passwordPtr)
    if ([string]::IsNullOrWhiteSpace($plainPassword)) {
        throw "POP3 password cannot be empty."
    }

    $bytes = [Text.Encoding]::UTF8.GetBytes($plainPassword)
    $protectedBytes = [System.Security.Cryptography.ProtectedData]::Protect(
        $bytes,
        $null,
        [System.Security.Cryptography.DataProtectionScope]::LocalMachine
    )
    [Convert]::ToBase64String($protectedBytes) | Set-Content -LiteralPath $secretFile -Encoding ASCII
}
finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($passwordPtr)
}

Write-Host "Saved machine-scoped POP3 password:"
Write-Host $secretFile
Write-Host "This file is intended for startup tasks running as NT AUTHORITY\SYSTEM on this computer."
