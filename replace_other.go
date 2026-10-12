//go:build !windows

package main

import "os"

// replaceFile renames tmp over path; rename(2) already has POSIX replace semantics.
func replaceFile(tmp, path string) error { return os.Rename(tmp, path) }
