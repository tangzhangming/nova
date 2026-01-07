#!/bin/bash
# Sola Sublime Text 插件自动安装脚本 (macOS/Linux)
# 使用方法: chmod +x install.sh && ./install.sh

echo "======================================"
echo "  Sola Sublime Text 插件安装程序"
echo "======================================"
echo ""

# 检测操作系统和 Sublime Text 路径
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS
    SUBLIME_PACKAGES="$HOME/Library/Application Support/Sublime Text/Packages"
    if [ ! -d "$SUBLIME_PACKAGES" ]; then
        SUBLIME_PACKAGES="$HOME/Library/Application Support/Sublime Text 3/Packages"
    fi
    OS_NAME="macOS"
else
    # Linux
    SUBLIME_PACKAGES="$HOME/.config/sublime-text/Packages"
    if [ ! -d "$SUBLIME_PACKAGES" ]; then
        SUBLIME_PACKAGES="$HOME/.config/sublime-text-3/Packages"
    fi
    OS_NAME="Linux"
fi

# 检查 Sublime Text 是否安装
if [ ! -d "$SUBLIME_PACKAGES" ]; then
    echo "✗ 错误: 未找到 Sublime Text 安装"
    echo "  请确保已安装 Sublime Text 3 或 4"
    exit 1
fi

echo "✓ 找到 Sublime Text 安装路径: $SUBLIME_PACKAGES"

# 创建 Sola 插件目录
SOLA_PACKAGE="$SUBLIME_PACKAGES/Sola"
if [ ! -d "$SOLA_PACKAGE" ]; then
    mkdir -p "$SOLA_PACKAGE"
    echo "✓ 创建插件目录: $SOLA_PACKAGE"
else
    echo "✓ 插件目录已存在: $SOLA_PACKAGE"
fi

# 复制文件
FILES=(
    "Sola.sublime-syntax"
    "Sola.sublime-completions"
    "Comments.tmPreferences"
)

echo ""
echo "开始复制文件..."

SUCCESS=0
FAILED=0

for file in "${FILES[@]}"; do
    if [ -f "$file" ]; then
        if cp "$file" "$SOLA_PACKAGE/"; then
            echo "  ✓ $file"
            ((SUCCESS++))
        else
            echo "  ✗ $file - 复制失败"
            ((FAILED++))
        fi
    else
        echo "  ✗ $file - 源文件不存在"
        ((FAILED++))
    fi
done

# 显示结果
echo ""
echo "======================================"
if [ $FAILED -eq 0 ]; then
    echo "安装完成！成功复制 $SUCCESS 个文件"
    echo ""
    echo "使用说明:"
    if [[ "$OSTYPE" == "darwin"* ]]; then
        echo "  1. 重启 Sublime Text"
        echo "  2. 打开 .sola 文件即可自动应用语法高亮"
        echo "  3. 使用 Cmd+/ 进行单行注释"
        echo "  4. 使用 Cmd+Shift+/ 进行块注释"
    else
        echo "  1. 重启 Sublime Text"
        echo "  2. 打开 .sola 文件即可自动应用语法高亮"
        echo "  3. 使用 Ctrl+/ 进行单行注释"
        echo "  4. 使用 Ctrl+Shift+/ 进行块注释"
    fi
    echo ""
    echo "更多信息请查看 README.md"
else
    echo "安装完成但有错误"
    echo "  成功: $SUCCESS 个文件"
    echo "  失败: $FAILED 个文件"
fi
echo "======================================"

