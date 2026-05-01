@echo off
setlocal
cd /d "%~dp0"

set "PS_EXE=pwsh.exe"
where pwsh.exe >nul 2>nul
if errorlevel 1 set "PS_EXE=powershell.exe"

"%PS_EXE%" -NoProfile -ExecutionPolicy Bypass -File "%~dp0install-startup-task.ps1" %*
set "EXITCODE=%ERRORLEVEL%"

echo.
if not "%EXITCODE%"=="0" (
    echo install-startup-task failed. Exit code: %EXITCODE%
) else (
    echo install-startup-task completed.
)
pause
exit /b %EXITCODE%
