@echo off
chcp 65001 >nul
:: stop-web helper (calls stop-web.ps1)
:: stop-web helper (calls stop-web.ps1)
:: stop-web helper (calls stop-web.ps1)
cd /d "%~dp0"

echo ==========================================
echo   AI Pixel - 关闭残留后台进程
echo ==========================================
echo.
echo   正在检测残留的 ai-pixel-analysis.exe 进程...
echo.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0stop-web.ps1"
if errorlevel 1 echo   [提示] 检测脚本执行异常，请检查 stop-web.ps1 是否在同目录。

echo.
echo 完成。按任意键退出...
pause >nul
