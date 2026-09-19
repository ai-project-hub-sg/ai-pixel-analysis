# ============================================================
# stop-web.ps1 - 查找并关闭所有残留的 ai-pixel-analysis.exe 进程
# 由 stop-web.bat 调用；单独运行也可以。
# 中文注释/中文输出在 PowerShell（按字符解析）下完全安全。
# ============================================================
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

# 找出所有 ai-pixel-analysis 进程（可能不止一个）
$procs = Get-Process ai-pixel-analysis -ErrorAction SilentlyContinue

if (-not $procs) {
    Write-Host "  未发现残留的 ai-pixel-analysis 进程，无需处理。"
} else {
    foreach ($p in $procs) {
        Write-Host ("  发现残留进程 PID=" + $p.Id + "，正在关闭...")
        try {
            Stop-Process -Id $p.Id -Force -ErrorAction Stop
            Write-Host ("  [成功] PID=" + $p.Id + " 已关闭") -ForegroundColor Green
        } catch {
            Write-Host ("  [失败] 无法关闭 PID=" + $p.Id + "，可能权限不足，请用管理员身份运行") -ForegroundColor Yellow
        }
    }
}

Write-Host ""
# 顺带报告 8080 端口状态（可能被别的程序占用，如实告知不静默）
if (Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue) {
    Write-Host "  注意：8080 端口仍被占用，可能是其他程序。" -ForegroundColor Yellow
} else {
    Write-Host "  8080 端口空闲。"
}
