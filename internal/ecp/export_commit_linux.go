//go:build linux

package ecp

import "golang.org/x/sys/unix"

func renameAuthorityExportNoReplace(source, target string) error {
	return unix.Renameat2(unix.AT_FDCWD, source, unix.AT_FDCWD, target, unix.RENAME_NOREPLACE)
}
