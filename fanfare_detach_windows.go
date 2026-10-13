//go:build windows

package main

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// detach: DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP, plus CREATE_BREAKAWAY_FROM_JOB when asked
// (a job object that forbids breakaway makes Start fail; the caller then retries without it).
func detach(cmd *exec.Cmd, breakaway bool) {
	flags := uint32(0x00000008 | 0x00000200)
	if breakaway {
		flags |= 0x01000000
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags}
}

// procStamp is the image name of a running pid ("" when it is not running).
func procStamp(pid int) string {
	b, err := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/FO", "CSV", "/NH").Output()
	if err != nil {
		return ""
	}
	f := strings.SplitN(strings.TrimSpace(string(b)), ",", 2)
	if len(f) < 2 {
		return ""
	}
	return strings.Trim(f[0], `"`)
}

func killPlayer(pid int) {
	_ = exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}
