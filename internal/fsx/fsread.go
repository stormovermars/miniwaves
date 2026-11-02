// Package fsx holds mw's platform file helpers: shared reads that never block a writer's
// rename on Windows, a non-blocking open, and an atomic replace.
package fsx

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"time"
)

// readTries and readBackoff bound the shared-read retry: 50 x 20 ms, like mw's renameRetry.
var (
	readTries   = 50
	readBackoff = 20 * time.Millisecond
)

// ReadFile reads a board, tenant, index, pid or hash file. On Windows the file is opened
// with FILE_SHARE_READ|WRITE|DELETE (a writer may rename over it while we read) and an open
// that still hits a sharing violation or ERROR_ACCESS_DENIED (a file replaced mid-rename, an
// antivirus scan) is retried; elsewhere it is a plain os.ReadFile.
func ReadFile(path string) ([]byte, error) {
	var err error
	for i := 0; i < readTries; i++ {
		var f *os.File
		if f, err = OpenShared(path); err == nil {
			b, rerr := io.ReadAll(f)
			f.Close()
			if rerr == nil || !transientShareErr(rerr) {
				return b, rerr
			}
			err = rerr
		} else if errors.Is(err, fs.ErrNotExist) || !transientShareErr(err) {
			return nil, err
		}
		if i+1 < readTries {
			time.Sleep(readBackoff)
		}
	}
	return nil, err
}

// Stat is os.Stat with the same retry on transient Windows sharing errors.
func Stat(path string) (fs.FileInfo, error) {
	var (
		fi  fs.FileInfo
		err error
	)
	for i := 0; i < readTries; i++ {
		if fi, err = os.Stat(path); err == nil || errors.Is(err, fs.ErrNotExist) || !transientShareErr(err) {
			return fi, err
		}
		if i+1 < readTries {
			time.Sleep(readBackoff)
		}
	}
	return fi, err
}
