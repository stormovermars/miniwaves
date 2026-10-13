//go:build !windows

package main

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// detach puts the player in its own session so it outlives the CLI (breakaway is Windows only).
func detach(cmd *exec.Cmd, _ bool) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

// procStamp identifies a running process by its start time; "" when it is not running.
func procStamp(pid int) string {
	b, err := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// killPlayer stops the player's session (it was started with Setsid, so pid is its group id).
func killPlayer(pid int) {
	if syscall.Kill(-pid, syscall.SIGTERM) != nil {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
}
