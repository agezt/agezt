// SPDX-License-Identifier: MIT

//go:build windows

package file

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// oNoFollow is 0 on Windows (O_NOFOLLOW is POSIX-only). Instead of the flag
// we provide openFileNoFollow, which opens a file ONCE and then resolves the
// final path of the opened handle by calling the Win32
// GetFinalPathNameByHandleW API directly. If the final path leaves the
// workspace root, the file is closed and an error returned — closing the
// symlink-swap TOCTOU between the containment check and the open (M427).
//
// Security note: Windows symlink/reparse-point creation is privileged
// (administrator or Developer Mode), so the TOCTOU window is narrower than
// on Unix — but directory junctions are creatable by non-admin users, and a
// malicious process running as the same user as the daemon could race the
// file tool. This guard closes that window.

var (
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	getFinalPathByHandle = kernel32.NewProc("GetFinalPathNameByHandleW")
)

// finalPathFromHandle resolves the real on-disk path of a file handle using
// the Win32 GetFinalPathNameByHandleW API. It follows through reparse points,
// junctions, and symlinks to return the canonical file system path.
// The VOLUME_NAME_DOS (0x0) flag returns a drive-letter path (C:\...).
func finalPathFromHandle(fd uintptr) (string, error) {
	// Start with a reasonable buffer; the API returns the required size
	// including NUL terminator if the buffer is too small.
	const startSize = 512
	for bufSize := startSize; ; bufSize *= 2 {
		buf := make([]uint16, bufSize)
		r, _, err := getFinalPathByHandle.Call(fd, uintptr(unsafe.Pointer(&buf[0])), uintptr(bufSize), 0)
		if r == 0 {
			// ERROR_MORE_DATA or other failure.
			if errno, ok := err.(syscall.Errno); ok && errno == syscall.ERROR_MORE_DATA {
				continue
			}
			return "", fmt.Errorf("GetFinalPathNameByHandleW: %w", err)
		}
		// r is the returned character count (with NUL terminator).
		return string(utf16.Decode(buf[:r])), nil
	}
}

// openFileNoFollow is a drop-in replacement for os.OpenFile with O_NOFOLLOW
// semantics on Windows: it opens the file and verifies the resulting handle
// still points inside the workspace root via the Win32 API.
func openFileNoFollow(path string, flag int, perm os.FileMode, workspaceRoot string) (*os.File, error) {
	f, err := os.OpenFile(path, flag, perm)
	if err != nil {
		return nil, err
	}

	// Resolve the final path through any reparse points / junctions / symlinks.
	finalPath, err := finalPathFromHandle(f.Fd())
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("openNoFollow: resolve final path: %w", err)
	}

	// GetFinalPathNameByHandle returns paths like
	// "\??\C:\Users\...\workspace\file" — strip the "\??\" prefix if present.
	finalPath = cleanWinFinalPath(finalPath)
	ws := cleanWinFinalPath(workspaceRoot)

	if !strings.HasPrefix(finalPath, ws) {
		f.Close()
		return nil, fmt.Errorf("openNoFollow: resolved path %q is outside workspace %q (symlink/reparse-point TOCTOU)", finalPath, ws)
	}

	return f, nil
}

// ensureOnce for the lazy DLL loading.
var _ = sync.Once{}

// cleanWinFinalPath normalizes a Windows path for containment comparison:
// lowercases the drive letter, converts forward slashes to backslashes,
// removes the \\?\ prefix, cleans via filepath.Clean, and strips trailing
// backslashes.
func cleanWinFinalPath(p string) string {
	// Strip the NT namespace prefix that GetFinalPathNameByHandle returns.
	p = strings.TrimPrefix(p, "\\\\?\\")
	p = strings.TrimPrefix(p, "\\??\\")
	p = filepath.Clean(p)
	p = strings.ToLower(p)
	p = strings.ReplaceAll(p, "/", "\\")
	p = strings.TrimRight(p, "\\")
	return p
}
