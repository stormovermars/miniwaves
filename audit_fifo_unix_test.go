//go:build !windows

package main

import (
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestNewsSummaryFIFO(t *testing.T) {
	newsBoard(t)
	os.Remove("summary.txt")
	if err := syscall.Mkfifo("summary.txt", 0o600); err != nil {
		t.Skip(err)
	}
	done := make(chan string, 1)
	go func() { done <- captureOutput(t, func() { runAll(t, []string{"news", "--job", "tiny", "-o", "-"}) }) }()
	select {
	case out := <-done:
		if strings.Contains(out, "## Summary") {
			t.Fatal("FIFO summary was rendered")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("news hangs on a FIFO summary")
	}
}
