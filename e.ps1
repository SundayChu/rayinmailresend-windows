param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]] $AppArgs
)

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

$exe = Join-Path $PSScriptRoot "rayinmailresend.exe"
$main = Join-Path $PSScriptRoot "main.go"

if (-not (Test-Path -LiteralPath $exe) -or ((Get-Item -LiteralPath $exe).LastWriteTimeUtc -lt (Get-Item -LiteralPath $main).LastWriteTimeUtc)) {
    go build -o $exe .
}

& $exe @AppArgs
exit $LASTEXITCODE
