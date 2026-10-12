//go:build !windows

package main

import "os"

func openShared(path string) (*os.File, error) { return os.Open(path) }

// transientShareErr: only Windows has sharing violations.
func transientShareErr(error) bool { return false }
