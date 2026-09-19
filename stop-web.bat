@echo off
rem Launcher: all logic lives in stop-web.ps1 (UTF-8 BOM, safe for Chinese comments).
rem This .bat stays pure ASCII so cmd byte-parsing can never misread it.
cd /d "%~dp0"
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0stop-web.ps1"
echo.
echo Done. Press any key to exit...
pause >nul
