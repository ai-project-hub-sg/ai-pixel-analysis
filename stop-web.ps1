# stop-web.ps1 - find and kill leftover ai-pixel-analysis.exe processes
$procs = Get-Process ai-pixel-analysis -ErrorAction SilentlyContinue
if (-not $procs) {
    Write-Host "  No leftover ai-pixel-analysis process found."
} else {
    foreach ($p in $procs) {
        Write-Host ("  Found leftover PID=" + $p.Id + ", stopping...")
        try {
            Stop-Process -Id $p.Id -Force -ErrorAction Stop
            Write-Host ("  [OK] PID=" + $p.Id + " stopped")
        } catch {
            Write-Host ("  [FAIL] cannot stop PID=" + $p.Id + " (try running as Administrator)")
        }
    }
}
Write-Host ""
if (Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue) {
    Write-Host "  Note: port 8080 still in use (may be another program)."
} else {
    Write-Host "  Port 8080 is free."
}
