@echo off
rem 启动 AI Pixel 数据分析 Web 界面（含数据同步功能）
rem 双击本脚本即可；需与 ai-pixel-analysis.exe、.env 同目录
rem -sync 会加载 .env 解密会话用于后台抓取；分析接口仍是只读。
cd /d "%~dp0"
echo Starting web UI on http://localhost:8080 ...
echo (sync enabled; keep this window open. Closing the web page offers keep-in-background or stop-service.)
ai-pixel-analysis.exe web -sync
pause
