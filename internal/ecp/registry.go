package ecp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const maxWorkspaceBindingBytes = 64 << 10

func workspaceBindingPath(stateDir, workspaceID string) (string, error) {
	if err := validateIdentifier(workspaceID, "workspace_id"); err != nil {
		return "", err
	}
	return filepath.Join(stateDir, "workspace-bindings", workspaceID+".json"), nil
}

func loadWorkspaceBinding(stateDir, workspaceID string) (WorkspaceBinding, error) {
	path, err := workspaceBindingPath(stateDir, workspaceID)
	if err != nil {
		return WorkspaceBinding{}, err
	}
	if err := verifyAuthorityDirectoryAncestors(stateDir, filepath.Dir(path)); err != nil {
		return WorkspaceBinding{}, newError(KindIntegrity, "WORKSPACE_BINDING_UNSAFE", "Workspace binding has an unsafe authority directory chain", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return WorkspaceBinding{}, newError(KindNotFound, "WORKSPACE_NOT_REGISTERED", "this Git Workspace has no ECP authority binding in the selected state directory", err)
		}
		return WorkspaceBinding{}, newError(KindRuntime, "WORKSPACE_BINDING_STAT_FAILED", "could not inspect Workspace binding", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !hasPrivateFilePermissions(info) || info.Size() > maxWorkspaceBindingBytes {
		return WorkspaceBinding{}, newError(KindIntegrity, "WORKSPACE_BINDING_UNSAFE", "Workspace binding must be a bounded regular private file", nil)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkspaceBinding{}, newError(KindRuntime, "WORKSPACE_BINDING_READ_FAILED", "could not read Workspace binding", err)
	}
	var binding WorkspaceBinding
	if err := decodeStrictJSON(data, &binding); err != nil {
		return WorkspaceBinding{}, newError(KindIntegrity, "WORKSPACE_BINDING_INVALID", "Workspace binding is invalid", err)
	}
	if binding.SchemaVersion != SchemaVersion || binding.WorkspaceID != workspaceID {
		return WorkspaceBinding{}, newError(KindIntegrity, "WORKSPACE_BINDING_IDENTITY_MISMATCH", "Workspace binding schema or identity does not match its path", nil)
	}
	if err := validateIdentifier(binding.ProjectID, "project_id"); err != nil {
		return WorkspaceBinding{}, err
	}
	if err := validateIdentifier(binding.AuthorityID, "authority_id"); err != nil {
		return WorkspaceBinding{}, err
	}
	if !isSHA256Digest(binding.InitialConfigDigest) || !isSHA256Digest(binding.InitialTruthDigest) || binding.Actor == "" || binding.Reason == "" || binding.Trust == "" || binding.CoreIdentity == "" {
		return WorkspaceBinding{}, newError(KindIntegrity, "WORKSPACE_BINDING_INCOMPLETE", "Workspace binding is missing required authority fields", nil)
	}
	return binding, nil
}

func createWorkspaceBinding(ctx context.Context, stateDir string, binding WorkspaceBinding) error {
	if err := requireAuthorityPlatform(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	path, err := workspaceBindingPath(stateDir, binding.WorkspaceID)
	if err != nil {
		return err
	}
	if err := ensureAuthorityDirectory(stateDir, filepath.Dir(path)); err != nil {
		return err
	}
	data, err := json.MarshalIndent(binding, "", "  ")
	if err != nil {
		return newError(KindRuntime, "WORKSPACE_BINDING_ENCODE_FAILED", "could not encode Workspace binding", err)
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return newError(KindConflict, "WORKSPACE_ALREADY_REGISTERED", "this Workspace already has an authority binding", err)
		}
		return newError(KindRuntime, "WORKSPACE_BINDING_CREATE_FAILED", "could not create Workspace binding", err)
	}
	removeOnError := true
	defer func() {
		_ = file.Close()
		if removeOnError {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return newError(KindRuntime, "WORKSPACE_BINDING_WRITE_FAILED", "could not write Workspace binding", err)
	}
	if err := file.Sync(); err != nil {
		return newError(KindRuntime, "WORKSPACE_BINDING_SYNC_FAILED", "could not sync Workspace binding", err)
	}
	if err := file.Close(); err != nil {
		return newError(KindRuntime, "WORKSPACE_BINDING_CLOSE_FAILED", "could not close Workspace binding", err)
	}
	removeOnError = false
	return nil
}
