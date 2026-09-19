@echo off
rem 启动 AI Pixel 数据分析 Web 界面
rem 双击本脚本即可；需与 ai-pixel-analysis.exe 同目录
cd /d "%~dp0"
echo Starting web UI on http://localhost:8080 ...
echo (keep this window open; close it to stop the server)
ai-pixel-analysis.exe web
pause
