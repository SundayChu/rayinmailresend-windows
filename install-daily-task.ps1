param(
    [string] $TaskName = "rayinmailresend-rayin-to-gmail",
    [string] $At = "08:00",
    [int] $PollSeconds = 60
)

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

$restartScript = Join-Path $PSScriptRoot "manual-restart-rayin-to-gmail.ps1"
$secretFile = Join-Path $PSScriptRoot "secrets\pop3_password.txt"

if (-not (Test-Path -LiteralPath $secretFile)) {
    throw "Missing encrypted POP3 password. Run .\save-pop3-secret.ps1 first."
}
if (-not (Test-Path -LiteralPath $restartScript)) {
    throw "Missing restart script: $restartScript"
}

$time = [datetime]::ParseExact($At, "HH:mm", [Globalization.CultureInfo]::InvariantCulture)
$powershell = Join-Path $env:WINDIR "System32\WindowsPowerShell\v1.0\powershell.exe"

$actionArgs = '-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "{0}" -PollSeconds {1} -Background' -f $restartScript, $PollSeconds
$action = New-ScheduledTaskAction -Execute $powershell -Argument $actionArgs -WorkingDirectory $PSScriptRoot
$dailyTrigger = New-ScheduledTaskTrigger -Daily -At $time
$logonTrigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$triggers = @($logonTrigger, $dailyTrigger)
$settings = New-ScheduledTaskSettingsSet -MultipleInstances IgnoreNew -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 5) -ExecutionTimeLimit (New-TimeSpan -Days 1)
$principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive -RunLevel Limited

Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $triggers -Settings $settings -Principal $principal -Force | Out-Null

Write-Host "Installed scheduled task: $TaskName"
Write-Host "Triggers: At Windows logon, and daily at $At"
Write-Host "Restart command: $powershell $actionArgs"
Write-Host "Logs: $(Join-Path $PSScriptRoot 'logs')"
