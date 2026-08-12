//go:build unix

package ecp

import "os"

func requireAuthorityPlatform() error { return authorityPlatformRequirement(true) }

func hasPrivateFilePermissions(info os.FileInfo) bool {
	return info.Mode().Perm()&0o077 == 0
}
