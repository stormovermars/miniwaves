//go:build windows

package main

import (
	"errors"
	"os"
	"syscall"
)

const errSharingViolation syscall.Errno = 32 // ERROR_SHARING_VIOLATION

// openShared opens path read-only and lets other processes read, write, delete and rename it
// meanwhile (os.Open omits FILE_SHARE_DELETE, so a reader would block a writer's rename).
func openShared(path string) (*os.File, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}

// transientShareErr: the errors a concurrent open, rename or scan of the file causes.
func transientShareErr(err error) bool {
	return errors.Is(err, errSharingViolation) || errors.Is(err, syscall.ERROR_ACCESS_DENIED)
}
