# ============================================================
# start-web.ps1 - AI Pixel 数据分析 Web 界面启动器
# 说明：
#   交互式询问 监听端口（默认 8080）与 是否启用数据同步（默认启用）。
#   直接回车 = 使用默认值；输入 q 可随时退出。
#   所有非法输入都会明确报错 + 给正确示例 + 重新询问，绝不静默跳过。
# 为什么逻辑放 ps1 而不是 bat：
#   cmd 按"字节"解析 .bat，UTF-8 中文注释的某些字节会被误判为命令分隔符，
#   出现 "...is not recognized" 假报错；PowerShell 按"字符"解析，
#   UTF-8 with BOM 的中文注释/字符串完全安全，这是彻底解法。
# ============================================================

# 强制控制台 UTF-8 输出，保证中文提示在窗口里显示正常
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

# 切换到脚本所在目录，确保能找到 ai-pixel-analysis.exe
Set-Location -LiteralPath $PSScriptRoot

if (-not (Test-Path ".\ai-pixel-analysis.exe")) {
    Write-Host "[错误] 当前目录找不到 ai-pixel-analysis.exe" -ForegroundColor Red
    Write-Host "请把本脚本放到与 exe 同目录后再运行。"
    Read-Host "按回车退出"
    exit 1
}

Write-Host "=========================================="
Write-Host "  AI Pixel - 启动数据分析 Web 界面"
Write-Host "=========================================="
Write-Host ""

# ---------------- 1) 监听端口 ----------------
# 校验规则：纯数字 + 范围 1-65535 + 未被其他程序占用
while ($true) {
    $port = Read-Host "请输入监听端口 [默认 8080，直接回车；输入 q 退出]"
    if ($port -eq "q") { Write-Host "已取消启动。"; Read-Host "按回车退出"; exit 0 }
    if ([string]::IsNullOrWhiteSpace($port)) { $port = "8080" }

    # 必须是纯数字
    if ($port -notmatch "^\d+$") {
        Write-Host "[输入错误] `"$port`" 不是有效端口，必须是 1-65535 的纯数字。" -ForegroundColor Yellow
        Write-Host "  正确示例: 8080   或   9000"
        continue
    }
    $portNum = [int]$port
    if ($portNum -lt 1 -or $portNum -gt 65535) {
        Write-Host "[输入错误] 端口 $portNum 必须在 1-65535 之间。" -ForegroundColor Yellow
        Write-Host "  正确示例: 8080   或   9000"
        continue
    }
    # 端口是否已被占用
    $inUse = Get-NetTCPConnection -LocalPort $portNum -State Listen -ErrorAction SilentlyContinue
    if ($inUse) {
        Write-Host "[端口被占用] $portNum 已被其他程序占用，请换一个端口。" -ForegroundColor Yellow
        Write-Host "  正确示例: 8081   或   9000"
        Write-Host "  (若想先关掉占用者，可另开窗口运行 stop-web.bat)"
        continue
    }
    break   # 通过全部校验
}

# ---------------- 2) 是否启用数据同步 ----------------
# 只认 Y/N/yes/no（大小写不敏感）；其他输入报错重问
$syncArg = ""
while ($true) {
    $sync = Read-Host "是否启用数据同步? [Y/n，默认 Y 直接回车；输入 q 退出]"
    if ($sync -eq "q") { Write-Host "已取消启动。"; Read-Host "按回车退出"; exit 0 }
    if ([string]::IsNullOrWhiteSpace($sync)) { $sync = "Y" }
    switch -Regex ($sync) {
        "^(y|yes)$" { $syncArg = "-sync"; break }
        "^(n|no)$"  { $syncArg = ""; break }
        default {
            Write-Host "[输入错误] `"$sync`" 无法识别，只能输入 Y 或 N。" -ForegroundColor Yellow
            Write-Host "  正确示例: Y   或   n   (或直接回车表示 Y；输入 q 退出)"
            continue
        }
    }
    break
}

# ---------------- 3) 汇总并启动 ----------------
$syncLabel = if ($syncArg -eq "-sync") { "已启用（后台抓取数据，需 .env）" } else { "未启用（纯只读分析）" }
Write-Host ""
Write-Host "------------------------------------------"
Write-Host "  启动配置:"
Write-Host "    地址  = http://localhost:$portNum"
Write-Host "    同步  = $syncLabel"
Write-Host "------------------------------------------"
Write-Host "  浏览器打开 http://localhost:$portNum 即可使用"
Write-Host "  关闭本窗口 = 停止服务（或在页面点「关闭界面」选择）"
Write-Host ""

# 前台启动：窗口关 = 服务停；web 子命令本身阻塞直到服务结束
& ".\ai-pixel-analysis.exe" web -addr ":$portNum" $syncArg

Write-Host ""
Write-Host "服务已停止。按回车退出..."
Read-Host | Out-Null
