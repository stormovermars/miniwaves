//go:build !windows

package fsx

import (
	"os"
	"syscall"
)

func OpenShared(path string) (*os.File, error) { return os.Open(path) }

// transientShareErr: only Windows has sharing violations.
func transientShareErr(error) bool { return false }

// OpenNoBlock opens a file for reading without blocking on a FIFO (a swap after the regular-file check).
func OpenNoBlock(p string) (*os.File, error) {
	return os.OpenFile(p, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
