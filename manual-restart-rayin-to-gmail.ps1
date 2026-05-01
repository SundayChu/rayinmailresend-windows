param(
    [int] $PollSeconds = 60,
    [switch] $Background
)

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

$root = $PSScriptRoot
$runner = Join-Path $root "run-rayin-to-gmail-unattended.ps1"
$exe = Join-Path $root "rayinmailresend.exe"
$secretFile = Join-Path $root "secrets\pop3_password.txt"
$lockFile = Join-Path $root "rayinmailresend.lock"
$logDir = Join-Path $root "logs"
$restartLog = Join-Path $logDir ("manual-restart-" + (Get-Date -Format "yyyyMMdd") + ".log")

New-Item -ItemType Directory -Force -Path $logDir | Out-Null

function Write-Step {
    param([string] $Message)

    $line = "{0} {1}" -f (Get-Date -Format "yyyy-MM-dd HH:mm:ss"), $Message
    Add-Content -LiteralPath $restartLog -Value $line -Encoding utf8

    if (-not $Background) {
        Write-Host $line
    }
}

function Get-RunnerProcesses {
    $runnerLeaf = Split-Path -Leaf $runner
    Get-CimInstance Win32_Process |
        Where-Object {
            $_.ProcessId -ne $PID -and
            $_.Name -match "^(powershell|pwsh)\.exe$" -and
            (
                $_.CommandLine -like "*$runnerLeaf*" -or
                $_.CommandLine -like "*$runner*"
            )
        }
}

try {
    if (-not (Test-Path -LiteralPath $runner)) {
        throw "Missing runner script: $runner"
    }
    if (-not (Test-Path -LiteralPath $exe)) {
        throw "Missing executable: $exe"
    }
    if (-not (Test-Path -LiteralPath $secretFile)) {
        throw "Missing encrypted POP3 password. Run save-pop3-secret.bat first."
    }

    if ($PollSeconds -le 0) {
        $PollSeconds = 60
    }

    Write-Step "Checking existing rayinmailresend processes."

    $runnerProcesses = @(Get-RunnerProcesses)
    foreach ($process in $runnerProcesses) {
        Write-Step "Stopping existing runner PID=$($process.ProcessId)."
        Stop-Process -Id $process.ProcessId -Force -ErrorAction SilentlyContinue
    }

    $appProcesses = @(Get-Process -Name "rayinmailresend" -ErrorAction SilentlyContinue)
    foreach ($process in $appProcesses) {
        Write-Step "Stopping existing app PID=$($process.Id)."
        Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
    }

    Start-Sleep -Seconds 1

    if (Test-Path -LiteralPath $lockFile) {
        Write-Step "Removing stale lock file."
        Remove-Item -LiteralPath $lockFile -Force -ErrorAction SilentlyContinue
    }

    $psExe = (Get-Command pwsh.exe -ErrorAction SilentlyContinue).Source
    if ([string]::IsNullOrWhiteSpace($psExe)) {
        $psExe = (Get-Command powershell.exe -ErrorAction Stop).Source
    }

    Write-Step "Starting unattended runner in background."
    $arguments = "-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File `"$runner`" -PollSeconds $PollSeconds"
    $started = Start-Process -FilePath $psExe -ArgumentList $arguments -WorkingDirectory $root -PassThru -WindowStyle Hidden

    Write-Step "Started runner PID=$($started.Id). Waiting for health check."
    Start-Sleep -Seconds 5

    $started.Refresh()
    if ($started.HasExited) {
        $latestLog = Get-ChildItem -LiteralPath $logDir -Filter "rayinmailresend-*.log" -File -ErrorAction SilentlyContinue |
            Sort-Object LastWriteTime -Descending |
            Select-Object -First 1

        if ($latestLog) {
            throw "Runner exited immediately. Check latest log: $($latestLog.FullName)"
        }
        throw "Runner exited immediately and no rayinmailresend log file was found."
    }

    $runningRunner = @(Get-RunnerProcesses)
    if ($runningRunner.Count -eq 0) {
        throw "Runner process was not found after startup."
    }

    Write-Step "Startup OK. Runner PID(s): $($runningRunner.ProcessId -join ', ')"
    Write-Step "Restart log: $restartLog"
    exit 0
}
catch {
    Write-Step "ERROR: $($_.Exception.Message)"
    exit 1
}
