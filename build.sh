#!/bin/bash
# Gensokyo 跨平台编译脚本

echo "开始编译 Gensokyo..."

# 编译 Linux amd64 版本
echo "编译 Linux amd64 版本..."
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o gensokyo_linux_amd64 .
if [ $? -eq 0 ]; then
    echo "✓ Linux amd64 编译成功"
else
    echo "✗ Linux amd64 编译失败"
    exit 1
fi

# 编译 Linux arm64 版本
echo "编译 Linux arm64 版本..."
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o gensokyo_linux_arm64 .
if [ $? -eq 0 ]; then
    echo "✓ Linux arm64 编译成功"
else
    echo "✗ Linux arm64 编译失败"
fi

# 编译 Windows amd64 版本
echo "编译 Windows amd64 版本..."
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H=windowsgui" -o gensokyo_windows_amd64.exe .
if [ $? -eq 0 ]; then
    echo "✓ Windows amd64 编译成功"
else
    echo "✗ Windows amd64 编译失败"
    exit 1
fi

echo ""
echo "编译完成！生成的文件："
ls -lh gensokyo_*

echo ""
echo "提示："
echo "- Linux 版本请使用 ./start_daemon.sh 启动（支持自动重启）"
echo "- Windows 版本请使用 gensokyo.bat 启动"
