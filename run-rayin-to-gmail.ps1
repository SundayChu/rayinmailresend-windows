param(
    [switch] $Once,
    [switch] $DryRun,
    [int] $PollSeconds = 60
)

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

$runScript = Join-Path $PSScriptRoot "run.ps1"

if ($Once -and $DryRun) {
    & $runScript -Username "sunday@rayin.com.tw" -To "java.sunday@gmail.com" -Pop3Host "mail.rayin.com.tw" -Pop3Port 110 -Pop3TlsMode "starttls" -PollSeconds $PollSeconds -Once -DryRun
}
elseif ($Once) {
    & $runScript -Username "sunday@rayin.com.tw" -To "java.sunday@gmail.com" -Pop3Host "mail.rayin.com.tw" -Pop3Port 110 -Pop3TlsMode "starttls" -PollSeconds $PollSeconds -Once
}
elseif ($DryRun) {
    & $runScript -Username "sunday@rayin.com.tw" -To "java.sunday@gmail.com" -Pop3Host "mail.rayin.com.tw" -Pop3Port 110 -Pop3TlsMode "starttls" -PollSeconds $PollSeconds -DryRun
}
else {
    & $runScript -Username "sunday@rayin.com.tw" -To "java.sunday@gmail.com" -Pop3Host "mail.rayin.com.tw" -Pop3Port 110 -Pop3TlsMode "starttls" -PollSeconds $PollSeconds
}
exit $LASTEXITCODE
