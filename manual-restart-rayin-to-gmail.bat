@echo off
setlocal
cd /d "%~dp0"

set "PS_EXE=pwsh.exe"
where pwsh.exe >nul 2>nul
if errorlevel 1 set "PS_EXE=powershell.exe"

set "NO_PAUSE="
set "BACKGROUND="
set "POLL_SECONDS=60"

:parse_args
if "%~1"=="" goto args_done
if /I "%~1"=="--no-pause" (
    set "NO_PAUSE=1"
    set "BACKGROUND=1"
    shift
    goto parse_args
)
if /I "%~1"=="/nopause" (
    set "NO_PAUSE=1"
    set "BACKGROUND=1"
    shift
    goto parse_args
)
if /I "%~1"=="--background" (
    set "BACKGROUND=1"
    shift
    goto parse_args
)
if /I "%~1"=="--poll-seconds" (
    if not "%~2"=="" set "POLL_SECONDS=%~2"
    shift
    shift
    goto parse_args
)
shift
goto parse_args

:args_done

set "APP_ARGS=-PollSeconds %POLL_SECONDS%"
if "%BACKGROUND%"=="1" set "APP_ARGS=%APP_ARGS% -Background"

"%PS_EXE%" -NoProfile -ExecutionPolicy Bypass -File "%~dp0manual-restart-rayin-to-gmail.ps1" %APP_ARGS%
set "EXITCODE=%ERRORLEVEL%"

if not "%NO_PAUSE%"=="1" (
    echo.
    if not "%EXITCODE%"=="0" (
        echo Manual restart failed. Exit code: %EXITCODE%
    ) else (
        echo Manual restart completed.
    )
    pause
)

exit /b %EXITCODE%
