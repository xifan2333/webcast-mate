package bilibili

import (
	"os/exec"
)

func lookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func newCmd(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}
