@echo off
chcp 65001 >nul
setlocal EnableDelayedExpansion
:: 启动 AI Pixel 数据分析 Web 界面
:: 交互式选择：端口（默认8080）、是否启用数据同步（默认启用）
:: 直接回车 = 用默认值。输入非法会提示并重输，不会静默跳过。
cd /d "%~dp0"

if not exist "ai-pixel-analysis.exe" (
  echo [错误] 当前目录找不到 ai-pixel-analysis.exe
  echo 请把本脚本放到与 exe 同目录后再运行。
  pause
  exit /b 1
)

echo ==========================================
echo   AI Pixel - 启动数据分析 Web 界面
echo ==========================================
echo.

:: ===== 1) 端口 =====
:: 注意：set /p 在变量已有值时回车会保留旧值，所以每轮先清空再询问。
:ask_port
set "PORT="
set /p "PORT=请输入监听端口 [默认 8080，直接回车；输入 q 退出]: "<con
if not defined PORT set "PORT=8080"
if /i "%PORT%"=="q" (echo 已取消启动。& pause & exit /b 0)
:: 校验：必须是纯数字（findstr 正则，^$ 锚定整行）
echo(%PORT%|findstr /r "^[0-9][0-9]*$" >nul
if errorlevel 1 (
  echo [输入错误] "%PORT%" 不是有效端口，必须是 1-65535 的纯数字。
  echo   正确示例: 8080   或   9000
  goto ask_port
)
:: 校验：范围 1-65535
if %PORT% LSS 1 (
  echo [输入错误] 端口 %PORT% 太小，必须在 1-65535 之间。
  echo   正确示例: 8080   或   9000
  goto ask_port
)
if %PORT% GTR 65535 (
  echo [输入错误] 端口 %PORT% 超出范围，必须在 1-65535 之间。
  echo   正确示例: 8080   或   9000
  goto ask_port
)
:: 校验：端口是否被占用
powershell -NoProfile -Command "if (Get-NetTCPConnection -LocalPort %PORT% -State Listen -ErrorAction SilentlyContinue) { exit 1 }" >nul 2>&1
if errorlevel 1 (
  echo [端口被占用] %PORT% 已被其他程序占用，请换一个端口。
  echo   正确示例: 8081   或   9000
  echo   ^(若想先关掉占用者，可另开窗口运行 stop-web.bat^)
  goto ask_port
)

:: ===== 2) 是否启用数据同步 =====
set "SYNCARG="
:ask_sync
set "SYNC="
set /p "SYNC=是否启用数据同步? [Y/n，默认 Y 直接回车；输入 q 退出]: "<con
if not defined SYNC set "SYNC=Y"
if /i "%SYNC%"=="q" (echo 已取消启动。& pause & exit /b 0)
if /i "%SYNC%"=="Y" set "SYNCARG=-sync" & goto sync_done
if /i "%SYNC%"=="yes" set "SYNCARG=-sync" & goto sync_done
if /i "%SYNC%"=="N" goto sync_done
if /i "%SYNC%"=="no" goto sync_done
echo [输入错误] "%SYNC%" 无法识别，只能输入 Y 或 N。
echo   正确示例: Y   或   n   ^(或直接回车表示 Y；输入 q 退出^)
goto ask_sync
:sync_done

echo.
echo ------------------------------------------
echo   启动配置:
echo     地址  = http://localhost:%PORT%
if "%SYNCARG%"=="-sync" (echo     同步  = 已启用（后台抓取数据，需 .env^)) else (echo     同步  = 未启用（纯只读分析）)
echo ------------------------------------------
echo   浏览器打开 http://localhost:%PORT% 即可使用
echo   关闭本窗口 = 停止服务（或在页面点「关闭界面」选择）
echo.
ai-pixel-analysis.exe web -addr :%PORT% %SYNCARG%
echo.
echo 服务已停止。按任意键退出...
pause >nul
