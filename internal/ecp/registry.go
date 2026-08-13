package ecp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
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
	release, err := acquirePlatformFileLock(
		ctx,
		filepath.Join(filepath.Dir(path), "."+binding.WorkspaceID+".registry.lock"),
		5*time.Second,
		"WORKSPACE_REGISTRY_LOCKED",
		"another Workspace registration or binding recovery is active",
	)
	if err != nil {
		return err
	}
	defer release()
	data, err := json.MarshalIndent(binding, "", "  ")
	if err != nil {
		return newError(KindRuntime, "WORKSPACE_BINDING_ENCODE_FAILED", "could not encode Workspace binding", err)
	}
	data = append(data, '\n')
	staged, err := stageWorkspaceBinding(filepath.Dir(path), data)
	if err != nil {
		return err
	}
	defer os.Remove(staged)
	return installWorkspaceBinding(stateDir, binding.WorkspaceID, staged, path)
}

func stageWorkspaceBinding(directory string, data []byte) (string, error) {
	file, err := os.CreateTemp(directory, ".binding-stage-")
	if err != nil {
		return "", newError(KindRuntime, "WORKSPACE_BINDING_STAGE_FAILED", "could not create Workspace binding staging file", err)
	}
	path := file.Name()
	removeOnError := true
	defer func() {
		_ = file.Close()
		if removeOnError {
			_ = os.Remove(path)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return "", newError(KindRuntime, "WORKSPACE_BINDING_CHMOD_FAILED", "could not secure Workspace binding staging file", err)
	}
	if _, err := file.Write(data); err != nil {
		return "", newError(KindRuntime, "WORKSPACE_BINDING_WRITE_FAILED", "could not write Workspace binding staging file", err)
	}
	if err := file.Sync(); err != nil {
		return "", newError(KindRuntime, "WORKSPACE_BINDING_SYNC_FAILED", "could not sync Workspace binding staging file", err)
	}
	if err := file.Close(); err != nil {
		return "", newError(KindRuntime, "WORKSPACE_BINDING_CLOSE_FAILED", "could not close Workspace binding staging file", err)
	}
	if err := syncDirectory(directory); err != nil {
		return "", newError(KindRuntime, "WORKSPACE_BINDING_DIRECTORY_SYNC_FAILED", "could not sync Workspace binding staging directory", err)
	}
	removeOnError = false
	return path, nil
}

func installWorkspaceBinding(stateDir, workspaceID, staged, target string) error {
	if err := os.Link(staged, target); err == nil {
		return finishWorkspaceBindingInstall(filepath.Dir(target), staged)
	} else if !errors.Is(err, os.ErrExist) {
		return newError(KindRuntime, "WORKSPACE_BINDING_INSTALL_FAILED", "could not atomically install Workspace binding", err)
	}

	recoverable, recoveryErr := recoverableWorkspaceBindingRemnant(stateDir, workspaceID)
	if recoveryErr != nil {
		return recoveryErr
	}
	if !recoverable {
		return newError(KindConflict, "WORKSPACE_ALREADY_REGISTERED", "this Workspace already has an authority binding or a non-recoverable binding record", os.ErrExist)
	}
	recoveryID, err := randomID("recovery", 8)
	if err != nil {
		return err
	}
	directory := filepath.Dir(target)
	quarantine := filepath.Join(directory, "."+workspaceID+"."+recoveryID+".truncated")
	if err := os.Rename(target, quarantine); err != nil {
		return newError(KindRuntime, "WORKSPACE_BINDING_RECOVERY_STAGE_FAILED", "could not isolate the truncated Workspace binding", err)
	}
	if err := syncDirectory(directory); err != nil {
		return newError(KindRuntime, "WORKSPACE_BINDING_DIRECTORY_SYNC_FAILED", "could not persist truncated binding isolation", err)
	}
	if err := os.Link(staged, target); err != nil {
		_ = os.Rename(quarantine, target)
		_ = syncDirectory(directory)
		if errors.Is(err, os.ErrExist) {
			return newError(KindConflict, "WORKSPACE_ALREADY_REGISTERED", "another registration installed a Workspace binding during recovery", err)
		}
		return newError(KindRuntime, "WORKSPACE_BINDING_INSTALL_FAILED", "could not install Workspace binding after isolating a truncated remnant", err)
	}
	if err := syncDirectory(directory); err != nil {
		return newError(KindRuntime, "WORKSPACE_BINDING_DIRECTORY_SYNC_FAILED", "could not persist recovered Workspace binding installation", err)
	}
	if err := os.Remove(quarantine); err != nil {
		return newError(KindRuntime, "WORKSPACE_BINDING_RECOVERY_CLEANUP_FAILED", "recovered Workspace binding but could not remove the isolated truncated remnant", err)
	}
	return finishWorkspaceBindingInstall(directory, staged)
}

func finishWorkspaceBindingInstall(directory, staged string) error {
	if err := syncDirectory(directory); err != nil {
		return newError(KindRuntime, "WORKSPACE_BINDING_DIRECTORY_SYNC_FAILED", "could not durably commit Workspace binding installation", err)
	}
	if err := os.Remove(staged); err != nil && !os.IsNotExist(err) {
		return newError(KindRuntime, "WORKSPACE_BINDING_STAGE_CLEANUP_FAILED", "Workspace binding was installed but its staging link could not be removed", err)
	}
	if err := syncDirectory(directory); err != nil {
		return newError(KindRuntime, "WORKSPACE_BINDING_DIRECTORY_SYNC_FAILED", "could not persist Workspace binding staging cleanup", err)
	}
	return nil
}

func recoverableWorkspaceBindingRemnant(stateDir, workspaceID string) (bool, error) {
	path, err := workspaceBindingPath(stateDir, workspaceID)
	if err != nil {
		return false, err
	}
	if err := verifyAuthorityDirectoryAncestors(stateDir, filepath.Dir(path)); err != nil {
		return false, newError(KindIntegrity, "WORKSPACE_BINDING_UNSAFE", "Workspace binding has an unsafe authority directory chain", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, newError(KindRuntime, "WORKSPACE_BINDING_STAT_FAILED", "could not inspect Workspace binding recovery candidate", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !hasPrivateFilePermissions(info) || info.Size() > maxWorkspaceBindingBytes {
		return false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, newError(KindRuntime, "WORKSPACE_BINDING_READ_FAILED", "could not read Workspace binding recovery candidate", err)
	}
	if !truncatedJSONObject(data) {
		return false, nil
	}
	historyExists, err := workspaceAuthorityHistoryExists(stateDir, workspaceID)
	if err != nil {
		return false, err
	}
	if historyExists {
		return false, newError(KindIntegrity, "WORKSPACE_BINDING_RECOVERY_REFUSED", "truncated Workspace binding has matching authority history and cannot be replaced automatically", nil)
	}
	return true, nil
}

func truncatedJSONObject(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return true
	}
	var value any
	err := json.Unmarshal(trimmed, &value)
	var syntax *json.SyntaxError
	return errors.As(err, &syntax) && syntax.Offset >= int64(len(trimmed))
}

func workspaceAuthorityHistoryExists(stateDir, workspaceID string) (bool, error) {
	projects := filepath.Join(stateDir, "projects")
	info, err := os.Lstat(projects)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, newError(KindRuntime, "WORKSPACE_BINDING_RECOVERY_SCAN_FAILED", "could not inspect authority projects during binding recovery", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !hasPrivateFilePermissions(info) {
		return false, newError(KindIntegrity, "WORKSPACE_BINDING_RECOVERY_SCAN_UNSAFE", "authority projects directory is unsafe during binding recovery", nil)
	}
	entries, err := os.ReadDir(projects)
	if err != nil {
		return false, newError(KindRuntime, "WORKSPACE_BINDING_RECOVERY_SCAN_FAILED", "could not enumerate authority projects during binding recovery", err)
	}
	for _, entry := range entries {
		entryInfo, err := entry.Info()
		if err != nil {
			return false, newError(KindRuntime, "WORKSPACE_BINDING_RECOVERY_SCAN_FAILED", "could not inspect an authority project during binding recovery", err)
		}
		if entryInfo.Mode()&os.ModeSymlink != 0 || !entryInfo.IsDir() || !hasPrivateFilePermissions(entryInfo) {
			return false, newError(KindIntegrity, "WORKSPACE_BINDING_RECOVERY_SCAN_UNSAFE", fmt.Sprintf("authority project entry %q is unsafe during binding recovery", entry.Name()), nil)
		}
		candidate := filepath.Join(projects, entry.Name(), "workspaces", workspaceID)
		if _, err := os.Lstat(candidate); err == nil {
			return true, nil
		} else if !os.IsNotExist(err) {
			return false, newError(KindRuntime, "WORKSPACE_BINDING_RECOVERY_SCAN_FAILED", "could not inspect matching Workspace authority history", err)
		}
	}
	return false, nil
}
