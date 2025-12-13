//go:build linux || darwin
// +build linux darwin

package sys

import (
	"os"
	"os/exec"
	"syscall"
)

// UnixRestarter implements the Restarter interface for Unix-like systems.
type UnixRestarter struct{}

// NewRestarter creates a new Restarter appropriate for Unix-like systems.
func NewRestarter() *UnixRestarter {
	return &UnixRestarter{}
}

// Restart restarts the application on Unix-like systems.
func (r *UnixRestarter) Restart(executableName string) error {
	// 获取可执行文件的绝对路径
	execPath, err := os.Executable()
	if err != nil {
		return err
	}

	// 获取工作目录
	workDir, err := os.Getwd()
	if err != nil {
		return err
	}

	// 创建重启脚本
	scriptContent := "#!/bin/bash\n" +
		"sleep 2\n" + // 等待主进程完全退出
		"cd \"" + workDir + "\"\n" + // 切换到工作目录
		"\"" + execPath + "\" -faststart >> gensokyo.log 2>&1 &\n" + // 后台运行并重定向日志
		"rm -f restart.sh\n" // 删除自己

	scriptName := "restart.sh"
	if err := os.WriteFile(scriptName, []byte(scriptContent), 0755); err != nil {
		return err
	}

	// 使用 sh 在后台运行脚本
	cmd := exec.Command("sh", "-c", "nohup sh "+scriptName+" > /dev/null 2>&1 &")
	cmd.Dir = workDir
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // 创建新的进程组
		Setsid:  true, // 创建新的会话，脱离当前终端
	}

	// 完全分离子进程
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return err
	}

	// 立即返回，不等待子进程
	return nil
}
