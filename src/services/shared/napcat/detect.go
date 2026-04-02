package napcat

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// FindNodeBinary locates a Node.js executable on the system.
func FindNodeBinary() (string, error) {
	if p, err := exec.LookPath("node"); err == nil {
		return p, nil
	}
	if runtime.GOOS == "windows" {
		candidates := []string{
			filepath.Join(os.Getenv("ProgramFiles"), "nodejs", "node.exe"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "nodejs", "node.exe"),
			filepath.Join(os.Getenv("APPDATA"), "nvm", "current", "node.exe"),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				return c, nil
			}
		}
	}
	return "", fmt.Errorf("node not found in PATH")
}
