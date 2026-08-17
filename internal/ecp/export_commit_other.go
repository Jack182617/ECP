//go:build !darwin && !linux

package ecp

func renameAuthorityExportNoReplace(_, _ string) error {
	return errAuthorityExportNoReplaceUnsupported
}
