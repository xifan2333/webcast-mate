package xiaohongshu

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func lookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func startDetached(bin, arg string) error {
	cmd := exec.Command(bin, arg)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	// detach
	return cmd.Start()
}

// unused helpers keep build happy if needed
func _abs(p string) string {
	a, _ := filepath.Abs(p)
	return a
}

func _envPath() []string {
	return strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))
}
