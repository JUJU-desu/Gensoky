@echo off
REM Gensokyo 跨平台编译脚本 (Windows版本)

echo 开始编译 Gensokyo...

REM 编译 Windows amd64 版本
echo 编译 Windows amd64 版本...
set GOOS=windows
set GOARCH=amd64
go build -ldflags="-s -w" -o gensokyo_windows_amd64.exe .
if %errorlevel% neq 0 (
    echo X Windows amd64 编译失败
    exit /b 1
)
echo √ Windows amd64 编译成功

REM 编译 Linux amd64 版本
echo 编译 Linux amd64 版本...
set GOOS=linux
set GOARCH=amd64
go build -ldflags="-s -w" -o gensokyo_linux_amd64 .
if %errorlevel% neq 0 (
    echo X Linux amd64 编译失败
    exit /b 1
)
echo √ Linux amd64 编译成功

REM 编译 Linux arm64 版本
echo 编译 Linux arm64 版本...
set GOOS=linux
set GOARCH=arm64
go build -ldflags="-s -w" -o gensokyo_linux_arm64 .
if %errorlevel% neq 0 (
    echo X Linux arm64 编译失败
)
echo √ Linux arm64 编译成功

echo.
echo 编译完成！
dir gensokyo_*

echo.
echo 提示：
echo - Linux 版本请使用 ./start_daemon.sh 启动（支持自动重启）
echo - Windows 版本请使用 gensokyo.bat 启动
pause
