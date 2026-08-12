//go:build !unix

package ecp

import "os"

func requireAuthorityPlatform() error {
	return authorityPlatformRequirement(false)
}

func hasPrivateFilePermissions(os.FileInfo) bool { return false }
