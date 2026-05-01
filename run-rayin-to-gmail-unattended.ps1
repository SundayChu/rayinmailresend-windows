param(
    [int] $PollSeconds = 60
)

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
$OutputEncoding = [Text.UTF8Encoding]::new($false)

$logDir = Join-Path $PSScriptRoot "logs"
$secretFile = Join-Path $PSScriptRoot "secrets\pop3_password.txt"
$machineSecretFile = Join-Path $PSScriptRoot "secrets\pop3_password.machine"
$lockFile = Join-Path $PSScriptRoot "rayinmailresend.lock"

New-Item -ItemType Directory -Force -Path $logDir | Out-Null

$logFile = Join-Path $logDir ("rayinmailresend-" + (Get-Date -Format "yyyyMMdd") + ".log")

function Write-Log {
    param([string] $Message)
    $line = "{0} {1}" -f (Get-Date -Format "yyyy-MM-dd HH:mm:ss"), $Message
    Add-Content -LiteralPath $logFile -Value $line -Encoding utf8
}

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

function Read-MachineSecret {
    param([string] $Path)

    Import-ProtectedData
    $encoded = (Get-Content -LiteralPath $Path -Raw).Trim()
    $protectedBytes = [Convert]::FromBase64String($encoded)
    $bytes = [System.Security.Cryptography.ProtectedData]::Unprotect(
        $protectedBytes,
        $null,
        [System.Security.Cryptography.DataProtectionScope]::LocalMachine
    )
    return [Text.Encoding]::UTF8.GetString($bytes)
}

if (-not (Test-Path -LiteralPath $machineSecretFile) -and -not (Test-Path -LiteralPath $secretFile)) {
    Write-Log "Missing POP3 password secret. Run save-pop3-machine-secret.bat for startup-without-logon mode."
    throw "Missing POP3 password secret. Run .\save-pop3-machine-secret.bat first."
}

if (Test-Path -LiteralPath $lockFile) {
    $existingPidText = (Get-Content -LiteralPath $lockFile -ErrorAction SilentlyContinue | Select-Object -First 1)
    $existingPid = 0
    if ([int]::TryParse($existingPidText, [ref] $existingPid)) {
        $existingProcess = Get-Process -Id $existingPid -ErrorAction SilentlyContinue
        if ($null -ne $existingProcess) {
            Write-Log "Already running with PID $existingPid. Exiting."
            exit 0
        }
    }
}

Set-Content -LiteralPath $lockFile -Value $PID -Encoding ASCII

try {
    if (Test-Path -LiteralPath $machineSecretFile) {
        $plainPassword = Read-MachineSecret -Path $machineSecretFile
    }
    else {
        $securePassword = Get-Content -LiteralPath $secretFile | ConvertTo-SecureString
        $passwordPtr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($securePassword)

        try {
            $plainPassword = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($passwordPtr)
        }
        finally {
            [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($passwordPtr)
        }
    }

    $env:POP3_HOST = "mail.rayin.com.tw"
    $env:POP3_PORT = "110"
    $env:POP3_TLS_MODE = "starttls"
    $env:POP3_USERNAME = "sunday@rayin.com.tw"
    $env:POP3_PASSWORD = $plainPassword
    $env:RESEND_TO = "java.sunday@gmail.com"
    $env:PYTHONIOENCODING = "utf-8"

    $exe = Join-Path $PSScriptRoot "rayinmailresend.exe"
    if (-not (Test-Path -LiteralPath $exe)) {
        Write-Log "Missing executable: $exe"
        throw "Missing executable: $exe"
    }

    Write-Log "Starting rayinmailresend unattended."
    & $exe --run --poll-seconds=$PollSeconds 2>&1 |
        ForEach-Object {
            $line = $_.ToString()
            Add-Content -LiteralPath $logFile -Value $line -Encoding utf8
            $line
        }
    $exitCode = $LASTEXITCODE
    Write-Log "rayinmailresend exited with code $exitCode."
    exit $exitCode
}
finally {
    Remove-Item Env:\POP3_HOST -ErrorAction SilentlyContinue
    Remove-Item Env:\POP3_PORT -ErrorAction SilentlyContinue
    Remove-Item Env:\POP3_TLS_MODE -ErrorAction SilentlyContinue
    Remove-Item Env:\POP3_USERNAME -ErrorAction SilentlyContinue
    Remove-Item Env:\POP3_PASSWORD -ErrorAction SilentlyContinue
    Remove-Item Env:\RESEND_TO -ErrorAction SilentlyContinue
    Remove-Item Env:\PYTHONIOENCODING -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $lockFile -ErrorAction SilentlyContinue
}
