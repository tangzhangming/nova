# Sola Sublime Text 插件自动安装脚本 (Windows)
# 使用方法: 在 PowerShell 中运行: .\install.ps1

Write-Host "======================================" -ForegroundColor Cyan
Write-Host "  Sola Sublime Text 插件安装程序" -ForegroundColor Cyan
Write-Host "======================================" -ForegroundColor Cyan
Write-Host ""

# 检测 Sublime Text 安装路径
$sublimePaths = @(
    "$env:APPDATA\Sublime Text",
    "$env:APPDATA\Sublime Text 3",
    "$env:APPDATA\Sublime Text 4"
)

$sublimePackages = $null
foreach ($path in $sublimePaths) {
    if (Test-Path "$path\Packages") {
        $sublimePackages = "$path\Packages"
        Write-Host "✓ 找到 Sublime Text 安装路径: $path" -ForegroundColor Green
        break
    }
}

if (-not $sublimePackages) {
    Write-Host "✗ 错误: 未找到 Sublime Text 安装" -ForegroundColor Red
    Write-Host "  请确保已安装 Sublime Text 3 或 4" -ForegroundColor Yellow
    exit 1
}

# 创建 Sola 插件目录
$solaPackage = Join-Path $sublimePackages "Sola"
if (-not (Test-Path $solaPackage)) {
    New-Item -ItemType Directory -Force -Path $solaPackage | Out-Null
    Write-Host "✓ 创建插件目录: $solaPackage" -ForegroundColor Green
} else {
    Write-Host "✓ 插件目录已存在: $solaPackage" -ForegroundColor Green
}

# 复制文件
$files = @(
    "Sola.sublime-syntax",
    "Sola.sublime-completions",
    "Comments.tmPreferences"
)

Write-Host ""
Write-Host "开始复制文件..." -ForegroundColor Cyan

$success = 0
$failed = 0

foreach ($file in $files) {
    $source = Join-Path $PSScriptRoot $file
    $dest = Join-Path $solaPackage $file
    
    if (Test-Path $source) {
        try {
            Copy-Item $source $dest -Force
            Write-Host "  ✓ $file" -ForegroundColor Green
            $success++
        } catch {
            Write-Host "  ✗ $file - 复制失败: $_" -ForegroundColor Red
            $failed++
        }
    } else {
        Write-Host "  ✗ $file - 源文件不存在" -ForegroundColor Red
        $failed++
    }
}

# 显示结果
Write-Host ""
Write-Host "======================================" -ForegroundColor Cyan
if ($failed -eq 0) {
    Write-Host "安装完成！成功复制 $success 个文件" -ForegroundColor Green
    Write-Host ""
    Write-Host "使用说明:" -ForegroundColor Yellow
    Write-Host "  1. 重启 Sublime Text" -ForegroundColor White
    Write-Host "  2. 打开 .sola 文件即可自动应用语法高亮" -ForegroundColor White
    Write-Host "  3. 使用 Ctrl+/ 进行单行注释" -ForegroundColor White
    Write-Host "  4. 使用 Ctrl+Shift+/ 进行块注释" -ForegroundColor White
    Write-Host ""
    Write-Host "更多信息请查看 README.md" -ForegroundColor Cyan
} else {
    Write-Host "安装完成但有错误" -ForegroundColor Yellow
    Write-Host "  成功: $success 个文件" -ForegroundColor Green
    Write-Host "  失败: $failed 个文件" -ForegroundColor Red
}
Write-Host "======================================" -ForegroundColor Cyan

