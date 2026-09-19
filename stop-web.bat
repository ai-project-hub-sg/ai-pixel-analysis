@echo off
chcp 65001 >nul
:: 检测并关闭残留的 ai-pixel-analysis 后台进程
:: 查找/关闭逻辑在同目录 stop-web.ps1（避免 cmd 嵌套 PowerShell 的引号转义问题）；
:: ps1 输出为英文是为防止无 BOM 脚本被按 ANSI 误读，这里用 echo 补中文说明。
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
