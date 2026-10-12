package main

import (
	"regexp"
	"runtime"
	"strings"
)

var (
	psSetup = regexp.MustCompile(`\$env:MW_SESSION=('[^']*'); \$env:MW_TENANT=('[^']*')`)
	winSep  = regexp.MustCompile(`\\([^n"])`)
)

// replacePath replaces p (and its JSON-escaped form) with tag; the symlink-resolved form
// (macOS /private/var for /var) must be replaced first, else "/private<DIR>" is left.
func replacePath(s, p, resolved, tag string) string {
	for _, x := range []string{resolved, p} {
		if x == "" {
			continue
		}
		s = strings.ReplaceAll(s, strings.ReplaceAll(x, `\`, `\\`), tag)
		s = strings.ReplaceAll(s, x, tag)
	}
	return s
}

// osNorm maps Windows output onto the Unix goldens: PowerShell setup lines become the
// POSIX ones and path separators after <DIR>/<HOME> become slashes. No-op elsewhere.
func osNorm(s string) string {
	if runtime.GOOS != "windows" {
		return s
	}
	s = strings.ReplaceAll(s, "Set-Location '", "cd '")
	s = psSetup.ReplaceAllString(s, "export MW_SESSION=$1 MW_TENANT=$2")
	s = strings.ReplaceAll(s, `\\`, "/")
	if t := strings.TrimSpace(s); !strings.HasPrefix(t, "{") && !strings.HasPrefix(t, "[") {
		s = winSep.ReplaceAllString(s, "/$1")
	}
	return s
}
