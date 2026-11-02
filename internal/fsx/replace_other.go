//go:build !windows

package fsx

import "os"

// Replace renames tmp over path; rename(2) already has POSIX replace semantics.
func Replace(tmp, path string) error { return os.Rename(tmp, path) }
