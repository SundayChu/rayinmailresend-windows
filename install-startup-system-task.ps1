param(
    [string] $TaskName = "rayinmailresend-rayin-to-gmail",
    [string] $At = "08:00",
    [int] $PollSeconds = 60
)

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principalCheck = [Security.Principal.WindowsPrincipal]::new($identity)
if (-not $principalCheck.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "Run this script as Administrator to install a startup task under NT AUTHORITY\SYSTEM."
}

$restartScript = Join-Path $PSScriptRoot "manual-restart-rayin-to-gmail.ps1"
$machineSecretFile = Join-Path $PSScriptRoot "secrets\pop3_password.machine"

if (-not (Test-Path -LiteralPath $machineSecretFile)) {
    throw "Missing machine-scoped POP3 password. Run .\save-pop3-machine-secret.bat first."
}
if (-not (Test-Path -LiteralPath $restartScript)) {
    throw "Missing restart script: $restartScript"
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

$principal = New-ScheduledTaskPrincipal `
    -UserId "NT AUTHORITY\SYSTEM" `
    -LogonType ServiceAccount `
    -RunLevel Highest

Register-ScheduledTask `
    -TaskName $TaskName `
    -Action $action `
    -Trigger $triggers `
    -Settings $settings `
    -Principal $principal `
    -Force | Out-Null

Start-ScheduledTask -TaskName $TaskName

Write-Host "Installed startup scheduled task: $TaskName"
Write-Host "Account: NT AUTHORITY\SYSTEM"
Write-Host "Triggers: At computer startup, and daily at $At"
Write-Host "Restart command: $powershell $actionArgs"
Write-Host "Started task once now."
Write-Host "Logs: $(Join-Path $PSScriptRoot 'logs')"
