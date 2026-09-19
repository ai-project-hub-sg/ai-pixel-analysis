@echo off
chcp 65001 >nul
rem 检测并关闭残留的 ai-pixel-analysis 后台进程
rem 双击本脚本即可；关闭后停留在本窗口，按任意键退出。
cd /d "%~dp0"

set "FOUND=0"
echo ==========================================
echo   AI Pixel - 关闭残留后台进程
echo ==========================================
echo.

rem wmic 在新版 Win11 已移除，改用 PowerShell 查询（更通用）
for /f "usebackq tokens=*" %%P in (`powershell -NoProfile -Command "Get-CimInstance Win32_Process -Filter \"Name='ai-pixel-analysis.exe'\" ^| ForEach-Object { $_.ProcessId }"`) do (
  set "FOUND=1"
  echo   发现残留进程 PID=%%P，正在关闭...
  taskkill /PID %%P /F >nul 2>&1
  if errorlevel 1 (
    echo   [失败] 无法关闭 PID=%%P，可能权限不足，请用管理员身份运行本脚本
  ) else (
    echo   [成功] PID=%%P 已关闭
  )
)

echo.
if "%FOUND%"=="0" (
  echo   未发现残留的 ai-pixel-analysis 进程，无需处理。
) else (
  powershell -NoProfile -Command "if (Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue) { exit 1 }" >nul 2>&1
  if errorlevel 1 (echo   注意：8080 端口仍被占用，可能是其他程序。) else (echo   8080 端口已释放。)
)
echo.
echo 完成。按任意键退出...
pause >nul
