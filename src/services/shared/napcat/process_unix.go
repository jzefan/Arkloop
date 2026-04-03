//go:build !windows

package napcat

import "syscall"

func isProcessAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
