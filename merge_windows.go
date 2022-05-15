//+build windows

package m3u8d

import (
	"os/exec"
	"syscall"
)

func setupCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}
}