param(
    [string] $TaskName = "rayinmailresend-rayin-to-gmail",
    [string] $At = "08:00",
    [int] $PollSeconds = 60,
    [string] $Account = ""
)

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

$restartScript = Join-Path $PSScriptRoot "manual-restart-rayin-to-gmail.ps1"
$secretFile = Join-Path $PSScriptRoot "secrets\pop3_password.txt"

if (-not (Test-Path -LiteralPath $secretFile)) {
    throw "Missing encrypted POP3 password. Run .\save-pop3-secret.bat first."
}
if (-not (Test-Path -LiteralPath $restartScript)) {
    throw "Missing restart script: $restartScript"
}

if ([string]::IsNullOrWhiteSpace($Account)) {
    $defaultAccount = [Security.Principal.WindowsIdentity]::GetCurrent().Name
    $inputAccount = Read-Host -Prompt "Windows account for scheduled task [$defaultAccount]"
    if ([string]::IsNullOrWhiteSpace($inputAccount)) {
        $Account = $defaultAccount
    }
    else {
        $Account = $inputAccount.Trim()
    }
}

$securePassword = Read-Host -Prompt "Windows password for $Account" -AsSecureString
$passwordPtr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($securePassword)

try {
    $plainPassword = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($passwordPtr)
    if ([string]::IsNullOrEmpty($plainPassword)) {
        throw "Windows account password is required for startup tasks that run before logon."
    }

    $time = [datetime]::ParseExact($At, "HH:mm", [Globalization.CultureInfo]::InvariantCulture)
    $powershell = Join-Path $env:WINDIR "System32\WindowsPowerShell\v1.0\powershell.exe"
    $actionArgs = '-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "{0}" -PollSeconds {1} -Background' -f $restartScript, $PollSeconds
    $action = New-ScheduledTaskAction -Execute $powershell -Argument $actionArgs -WorkingDirectory $PSScriptRoot

    $startupTrigger = New-ScheduledTaskTrigger -AtStartup
    $dailyTrigger = New-ScheduledTaskTrigger -Daily -At $time
    $triggers = @($startupTrigger, $dailyTrigger)

    $settings = New-ScheduledTaskSettingsSet `
        -MultipleInstances IgnoreNew `
        -RestartCount 3 `
        -RestartInterval (New-TimeSpan -Minutes 5) `
        -ExecutionTimeLimit (New-TimeSpan -Days 1) `
        -StartWhenAvailable

    Register-ScheduledTask `
        -TaskName $TaskName `
        -Action $action `
        -Trigger $triggers `
        -Settings $settings `
        -User $Account `
        -Password $plainPassword `
        -RunLevel Limited `
        -Force | Out-Null
}
finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($passwordPtr)
}

Write-Host "Installed startup scheduled task: $TaskName"
Write-Host "Account: $Account"
Write-Host "Triggers: At computer startup, and daily at $At"
Write-Host "Restart command: $powershell $actionArgs"
Write-Host "Logs: $(Join-Path $PSScriptRoot 'logs')"
