//go:build windows

package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

// fileRenameInfo mirrors FILE_RENAME_INFO; in the Ex variant (FileRenameInfoEx) the
// ReplaceIfExists BOOLEAN becomes a DWORD Flags at the same offset.
type fileRenameInfo struct {
	Flags          uint32
	RootDirectory  windows.Handle
	FileNameLength uint32 // bytes, without the terminating NUL
	FileName       [1]uint16
}

const fileRenameFlagsPosixReplace = windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS // 0x3

// posixRenameOff is set once the POSIX rename proved unsupported (pre-1809 Windows, FAT, some
// network shares): later renames go straight to MoveFileEx. Tests set it to force the fallback.
var posixRenameOff atomic.Bool

// replaceFile renames tmp over path. MoveFileEx(REPLACE_EXISTING) (os.Rename) cannot replace a
// target that another process holds open, even with FILE_SHARE_DELETE ("Access is denied").
// The POSIX rename (SetFileInformationByHandle, FileRenameInfoEx, REPLACE_IF_EXISTS|POSIX_SEMANTICS)
// can: open readers keep the old file's contents and the name points at the new file at once.
// When the filesystem or OS does not support it, fall back to os.Rename.
func replaceFile(tmp, path string) error {
	if posixRenameOff.Load() {
		return os.Rename(tmp, path)
	}
	err := posixRename(tmp, path)
	switch {
	case err == nil || errors.Is(err, fs.ErrNotExist):
		return err
	case posixUnsupported(err):
		posixRenameOff.Store(true)
		return os.Rename(tmp, path)
	}
	// any other failure (a reader without FILE_SHARE_DELETE, a scan): MoveFileEx is never worse,
	// and the caller's retry loop waits out what both refuse
	if os.Rename(tmp, path) == nil {
		return nil
	}
	return err
}

func posixUnsupported(err error) bool {
	return errors.Is(err, windows.ERROR_INVALID_PARAMETER) || errors.Is(err, windows.ERROR_NOT_SUPPORTED) ||
		errors.Is(err, windows.ERROR_INVALID_FUNCTION) || errors.Is(err, windows.ERROR_CALL_NOT_IMPLEMENTED)
}

func posixRename(tmp, path string) error {
	lerr := func(err error) error { return &os.LinkError{Op: "rename", Old: tmp, New: path, Err: err} }
	abs, err := filepath.Abs(path)
	if err != nil {
		return lerr(err)
	}
	src, err := windows.UTF16PtrFromString(tmp)
	if err != nil {
		return lerr(err)
	}
	name, err := windows.UTF16FromString(abs)
	if err != nil {
		return lerr(err)
	}
	h, err := windows.CreateFile(src, windows.DELETE|windows.SYNCHRONIZE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return lerr(err)
	}
	defer windows.CloseHandle(h)
	off := unsafe.Offsetof(fileRenameInfo{}.FileName)
	size := off + uintptr(len(name))*2 // name includes the NUL
	buf := make([]uint64, (size+7)/8)  // 8-byte aligned for the HANDLE field
	info := (*fileRenameInfo)(unsafe.Pointer(&buf[0]))
	info.Flags = fileRenameFlagsPosixReplace
	info.FileNameLength = uint32(len(name)-1) * 2
	copy(unsafe.Slice(&info.FileName[0], len(name)), name)
	if err := windows.SetFileInformationByHandle(h, windows.FileRenameInfoEx, (*byte)(unsafe.Pointer(info)), uint32(size)); err != nil {
		return lerr(err)
	}
	return nil
}
