// Guest Agent — 编译后置于 microVM rootfs 内 /usr/local/bin/sandbox-agent
//
// 构建命令（在宿主机上交叉编译为静态二进制）：
//   GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o sandbox-agent ./agent
//
// rootfs 启动时通过 init 系统（OpenRC/busybox init/systemd）自动运行本程序。
// 也可配置为 kernel 的 init 进程：
//   kernel_args: "... init=/usr/local/bin/sandbox-agent"
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/mdlayher/vsock"
)

const (
	listenPort   = 8080
	maxCodeBytes = 4 * 1024 * 1024 // 4 MB
)

type ExecJob struct {
	Language  string `json:"language"`   // "python" | "shell"
	Code      string `json:"code"`
	TimeoutMs int    `json:"timeout_ms"`
}

type ExecResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	l, err := vsock.Listen(listenPort, nil)
	if err != nil {
		return fmt.Errorf("vsock listen :%d: %w", listenPort, err)
	}
	defer l.Close()

	fmt.Fprintf(os.Stderr, "sandbox-agent listening on vsock port %d\n", listenPort)

	for {
		conn, err := l.Accept()
		if err != nil {
			// listener 关闭时退出
			return nil
		}
		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()

	var job ExecJob
	if err := json.NewDecoder(conn).Decode(&job); err != nil {
		writeResult(conn, ExecResult{Stderr: fmt.Sprintf("decode job: %v", err), ExitCode: 1})
		return
	}

	result := executeJob(job)
	writeResult(conn, result)
}

func executeJob(job ExecJob) ExecResult {
	timeout := time.Duration(job.TimeoutMs) * time.Millisecond
	if timeout <= 0 || timeout > 5*time.Minute {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var cmd *exec.Cmd
	switch job.Language {
	case "python":
		cmd = buildPythonCmd(ctx, job.Code)
	case "shell":
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", job.Code)
	default:
		return ExecResult{Stderr: fmt.Sprintf("unsupported language: %q", job.Language), ExitCode: 1}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	return ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}
}

// buildPythonCmd 将代码写入临时文件后执行，避免 -c 参数引号转义问题。
func buildPythonCmd(ctx context.Context, code string) *exec.Cmd {
	f, err := os.CreateTemp("", "exec-*.py")
	if err != nil {
		// 降级为 -c 模式
		return exec.CommandContext(ctx, "python3", "-c", code)
	}
	_, _ = f.WriteString(code)
	_ = f.Close()

	cmd := exec.CommandContext(ctx, "python3", f.Name())
	// 执行后清理临时文件
	go func() {
		<-ctx.Done()
		_ = os.Remove(f.Name())
	}()
	return cmd
}

func writeResult(conn net.Conn, result ExecResult) {
	_ = json.NewEncoder(conn).Encode(result)
}
