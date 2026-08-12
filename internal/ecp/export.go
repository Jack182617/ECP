package ecp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	authorityExportFormat                 = "ecp-authority-export-v1"
	maxAuthorityExportManifestBytes       = 64 << 20
	maxAuthorityExportFiles               = 200000
	maxAuthorityExportBytes         int64 = 256 << 30
	maxEvidenceArtifactBytes        int64 = 64 << 20
)

const (
	exportRoleWorkspaceBinding = "workspace-binding"
	exportRoleEventRoot        = "event-root"
	exportRoleEventSegment     = "event-segment"
	exportRoleTruthBlob        = "truth-blob"
	exportRoleEvidenceArtifact = "evidence-artifact"
)

type AuthorityExportFile struct {
	Path      string `json:"path"`
	Role      string `json:"role"`
	SizeBytes int64  `json:"size_bytes"`
	Digest    string `json:"digest"`
}

type AuthorityExportManifest struct {
	SchemaVersion        int                   `json:"schema_version"`
	Format               string                `json:"format"`
	ProjectID            string                `json:"project_id"`
	AuthorityID          string                `json:"authority_id"`
	WorkspaceID          string                `json:"workspace_id"`
	Revision             uint64                `json:"revision"`
	EventHead            string                `json:"event_head"`
	Enabled              bool                  `json:"enabled"`
	ActivationID         string                `json:"activation_id,omitempty"`
	AcceptedConfigDigest string                `json:"accepted_config_digest"`
	AcceptedTruthDigest  string                `json:"accepted_truth_digest"`
	ContainsEvidenceLogs bool                  `json:"contains_evidence_logs"`
	Files                []AuthorityExportFile `json:"files"`
	BundleDigest         string                `json:"bundle_digest"`
}

type AuthorityExportResult struct {
	SchemaVersion        int    `json:"schema_version"`
	Path                 string `json:"path"`
	BundleDigest         string `json:"bundle_digest"`
	ProjectID            string `json:"project_id"`
	AuthorityID          string `json:"authority_id"`
	WorkspaceID          string `json:"workspace_id"`
	Revision             uint64 `json:"revision"`
	EventHead            string `json:"event_head"`
	FileCount            int    `json:"file_count"`
	TotalBytes           int64  `json:"total_bytes"`
	ContainsEvidenceLogs bool   `json:"contains_evidence_logs"`
	Trust                string `json:"trust"`
	Warning              string `json:"warning"`
}

type AuthorityVerifyResult struct {
	SchemaVersion        int    `json:"schema_version"`
	Path                 string `json:"path"`
	Valid                bool   `json:"valid"`
	BundleDigest         string `json:"bundle_digest"`
	ProjectID            string `json:"project_id"`
	AuthorityID          string `json:"authority_id"`
	WorkspaceID          string `json:"workspace_id"`
	Revision             uint64 `json:"revision"`
	EventHead            string `json:"event_head"`
	FileCount            int    `json:"file_count"`
	TotalBytes           int64  `json:"total_bytes"`
	ContainsEvidenceLogs bool   `json:"contains_evidence_logs"`
	Trust                string `json:"trust"`
	Warning              string `json:"warning"`
}

type authorityExportSource struct {
	path   string
	role   string
	source string
}

func (s Service) ExportAuthority(ctx context.Context, start, output string) (AuthorityExportResult, error) {
	service, workspace, err := s.loadAuthority(ctx, start)
	if err != nil {
		return AuthorityExportResult{}, err
	}
	target, err := resolveAuthorityExportTarget(workspace.Root, service.StateDir, output)
	if err != nil {
		return AuthorityExportResult{}, err
	}
	release, err := workspace.Store.acquireLock(ctx)
	if err != nil {
		return AuthorityExportResult{}, err
	}
	defer release()

	projection, err := workspace.Store.Load(ctx)
	if err != nil {
		return AuthorityExportResult{}, err
	}
	binding, err := loadWorkspaceBinding(service.StateDir, workspace.WorkspaceID)
	if err != nil {
		return AuthorityExportResult{}, err
	}
	if binding.ProjectID != workspace.ProjectID || binding.AuthorityID != service.authorityID {
		return AuthorityExportResult{}, newError(KindIntegrity, "WORKSPACE_BINDING_IDENTITY_MISMATCH", "Workspace binding changed before authority export", nil)
	}
	sources, err := collectAuthorityExportSources(ctx, service, workspace, projection)
	if err != nil {
		return AuthorityExportResult{}, err
	}

	parent := filepath.Dir(target)
	staging, err := os.MkdirTemp(parent, ".ecp-export-")
	if err != nil {
		return AuthorityExportResult{}, newError(KindRuntime, "EXPORT_STAGE_CREATE_FAILED", "could not create authority export staging directory", err)
	}
	removeStaging := true
	defer func() {
		if removeStaging {
			removeAuthorityExportTree(staging)
		}
	}()
	if err := os.Chmod(staging, 0o700); err != nil {
		return AuthorityExportResult{}, newError(KindRuntime, "EXPORT_STAGE_CHMOD_FAILED", "could not secure authority export staging directory", err)
	}

	files := make([]AuthorityExportFile, 0, len(sources))
	var total int64
	for _, source := range sources {
		select {
		case <-ctx.Done():
			return AuthorityExportResult{}, ctx.Err()
		default:
		}
		entry, err := copyAuthorityExportFile(source, staging)
		if err != nil {
			return AuthorityExportResult{}, err
		}
		total += entry.SizeBytes
		if total > maxAuthorityExportBytes {
			return AuthorityExportResult{}, newError(KindIntegrity, "EXPORT_TOO_LARGE", "authority export exceeds the supported aggregate size", nil)
		}
		files = append(files, entry)
	}
	manifest := AuthorityExportManifest{
		SchemaVersion:        SchemaVersion,
		Format:               authorityExportFormat,
		ProjectID:            workspace.ProjectID,
		AuthorityID:          service.authorityID,
		WorkspaceID:          workspace.WorkspaceID,
		Revision:             projection.Revision,
		EventHead:            projection.EventHead,
		Enabled:              projection.ECPEnabled(),
		AcceptedConfigDigest: projection.AcceptedConfig.ConfigDigest,
		AcceptedTruthDigest:  projection.AcceptedTruth.TruthDigest,
		ContainsEvidenceLogs: containsExportRole(files, exportRoleEvidenceArtifact),
		Files:                files,
	}
	if projection.Activation != nil {
		manifest.ActivationID = projection.Activation.ActivationID
	}
	manifest.BundleDigest, err = authorityExportManifestDigest(manifest)
	if err != nil {
		return AuthorityExportResult{}, newError(KindRuntime, "EXPORT_DIGEST_FAILED", "could not digest authority export manifest", err)
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return AuthorityExportResult{}, newError(KindRuntime, "EXPORT_MANIFEST_ENCODE_FAILED", "could not encode authority export manifest", err)
	}
	manifestBytes = append(manifestBytes, '\n')
	if len(manifestBytes) > maxAuthorityExportManifestBytes {
		return AuthorityExportResult{}, newError(KindIntegrity, "EXPORT_MANIFEST_TOO_LARGE", "authority export manifest exceeds the safety limit", nil)
	}
	if err := atomicWriteFile(filepath.Join(staging, "manifest.json"), manifestBytes, 0o600); err != nil {
		return AuthorityExportResult{}, err
	}
	verified, err := VerifyAuthorityExport(ctx, staging)
	if err != nil {
		return AuthorityExportResult{}, newError(KindIntegrity, "EXPORT_SELF_VERIFY_FAILED", "new authority export did not verify before commit", err)
	}
	if verified.BundleDigest != manifest.BundleDigest {
		return AuthorityExportResult{}, newError(KindIntegrity, "EXPORT_SELF_VERIFY_FAILED", "new authority export digest changed during verification", nil)
	}
	if err := sealAuthorityExport(staging); err != nil {
		return AuthorityExportResult{}, err
	}
	if err := os.Rename(staging, target); err != nil {
		return AuthorityExportResult{}, newError(KindRuntime, "EXPORT_COMMIT_FAILED", "could not atomically commit authority export", err)
	}
	removeStaging = false
	if directory, openErr := os.Open(parent); openErr == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return exportResultFromVerification(target, verified), nil
}

func VerifyAuthorityExport(ctx context.Context, bundlePath string) (AuthorityVerifyResult, error) {
	root, err := openAuthorityExportRoot(bundlePath)
	if err != nil {
		return AuthorityVerifyResult{}, err
	}
	manifest, err := readAuthorityExportManifest(root)
	if err != nil {
		return AuthorityVerifyResult{}, err
	}
	if err := validateAuthorityExportManifest(manifest); err != nil {
		return AuthorityVerifyResult{}, err
	}
	digest, err := authorityExportManifestDigest(manifest)
	if err != nil {
		return AuthorityVerifyResult{}, newError(KindRuntime, "EXPORT_DIGEST_FAILED", "could not digest authority export manifest", err)
	}
	if digest != manifest.BundleDigest {
		return AuthorityVerifyResult{}, newError(KindIntegrity, "EXPORT_BUNDLE_DIGEST_MISMATCH", "authority export manifest digest does not match", nil)
	}

	actual, total, err := inspectAuthorityExportFiles(ctx, root)
	if err != nil {
		return AuthorityVerifyResult{}, err
	}
	if len(actual) != len(manifest.Files) {
		return AuthorityVerifyResult{}, newError(KindIntegrity, "EXPORT_LAYOUT_INVALID", "authority export file set does not match its manifest", nil)
	}
	for index, expected := range manifest.Files {
		observed := actual[index]
		if observed.Path != expected.Path || observed.Role != expected.Role {
			return AuthorityVerifyResult{}, newError(KindIntegrity, "EXPORT_LAYOUT_INVALID", "authority export file identity does not match its manifest", nil)
		}
		if observed.SizeBytes != expected.SizeBytes {
			return AuthorityVerifyResult{}, newError(KindIntegrity, "EXPORT_FILE_SIZE_MISMATCH", fmt.Sprintf("authority export file %q size does not match", expected.Path), nil)
		}
		if observed.Digest != expected.Digest {
			return AuthorityVerifyResult{}, newError(KindIntegrity, "EXPORT_FILE_DIGEST_MISMATCH", fmt.Sprintf("authority export file %q digest does not match", expected.Path), nil)
		}
	}

	authorityDir := filepath.Join(root, "authority")
	binding, err := readExportedWorkspaceBinding(filepath.Join(authorityDir, "workspace-binding.json"), manifest.WorkspaceID)
	if err != nil {
		return AuthorityVerifyResult{}, err
	}
	if binding.ProjectID != manifest.ProjectID || binding.AuthorityID != manifest.AuthorityID {
		return AuthorityVerifyResult{}, newError(KindIntegrity, "EXPORT_IDENTITY_MISMATCH", "exported Workspace binding does not match the manifest", nil)
	}
	bundleStore := &Store{
		baseDir:     authorityDir,
		projectID:   manifest.ProjectID,
		workspaceID: manifest.WorkspaceID,
		authorityID: manifest.AuthorityID,
		dir:         authorityDir,
	}
	history, err := bundleStore.loadEventHistory(ctx, false)
	if err != nil {
		return AuthorityVerifyResult{}, err
	}
	projection := history.projection
	if projection.Revision != manifest.Revision || projection.EventHead != manifest.EventHead || projection.ECPEnabled() != manifest.Enabled {
		return AuthorityVerifyResult{}, newError(KindIntegrity, "EXPORT_PROJECTION_MISMATCH", "exported event projection does not match the manifest", nil)
	}
	if projection.AcceptedConfig == nil || projection.AcceptedConfig.ConfigDigest != manifest.AcceptedConfigDigest || projection.AcceptedTruth == nil || projection.AcceptedTruth.TruthDigest != manifest.AcceptedTruthDigest {
		return AuthorityVerifyResult{}, newError(KindIntegrity, "EXPORT_PROJECTION_MISMATCH", "exported accepted epochs do not match the manifest", nil)
	}
	activationID := ""
	if projection.Activation != nil {
		activationID = projection.Activation.ActivationID
	}
	if activationID != manifest.ActivationID {
		return AuthorityVerifyResult{}, newError(KindIntegrity, "EXPORT_PROJECTION_MISMATCH", "exported activation epoch does not match the manifest", nil)
	}
	for _, acceptance := range projection.TruthHistory {
		if _, err := bundleStore.ReadTruthFiles(acceptance.Files); err != nil {
			return AuthorityVerifyResult{}, err
		}
	}
	for _, changeID := range projection.ChangeOrder {
		for _, evidence := range projection.Evidence[changeID] {
			stdoutSize, stderrSize, err := expectedEvidenceArtifactSizes(projection, evidence)
			if err != nil {
				return AuthorityVerifyResult{}, err
			}
			if err := bundleStore.VerifyArtifact(evidence.StdoutArtifact, evidence.StdoutStoredDigest, stdoutSize); err != nil {
				return AuthorityVerifyResult{}, err
			}
			if err := bundleStore.VerifyArtifact(evidence.StderrArtifact, evidence.StderrStoredDigest, stderrSize); err != nil {
				return AuthorityVerifyResult{}, err
			}
		}
	}
	if err := requireExactReferencedExportSet(manifest, projection); err != nil {
		return AuthorityVerifyResult{}, err
	}
	return AuthorityVerifyResult{
		SchemaVersion:        SchemaVersion,
		Path:                 root,
		Valid:                true,
		BundleDigest:         manifest.BundleDigest,
		ProjectID:            manifest.ProjectID,
		AuthorityID:          manifest.AuthorityID,
		WorkspaceID:          manifest.WorkspaceID,
		Revision:             manifest.Revision,
		EventHead:            manifest.EventHead,
		FileCount:            len(manifest.Files),
		TotalBytes:           total,
		ContainsEvidenceLogs: manifest.ContainsEvidenceLogs,
		Trust:                "local-content-verified-export",
		Warning:              authorityExportWarning(),
	}, nil
}

func collectAuthorityExportSources(ctx context.Context, service Service, workspace loadedWorkspace, projection Projection) ([]authorityExportSource, error) {
	if projection.AcceptedConfig == nil || projection.AcceptedTruth == nil {
		return nil, newError(KindIntegrity, "EXPORT_ACCEPTED_EPOCH_MISSING", "authority export requires accepted control and Project Truth epochs", nil)
	}
	sources := map[string]authorityExportSource{}
	add := func(path, role, source string) error {
		if existing, ok := sources[path]; ok {
			if existing.role != role || existing.source != source {
				return newError(KindIntegrity, "EXPORT_SOURCE_COLLISION", "authority export source paths collide", nil)
			}
			return nil
		}
		if len(sources) >= maxAuthorityExportFiles {
			return newError(KindIntegrity, "EXPORT_TOO_MANY_FILES", "authority export exceeds the file-count safety limit", nil)
		}
		sources[path] = authorityExportSource{path: path, role: role, source: source}
		return nil
	}
	bindingPath, err := workspaceBindingPath(service.StateDir, workspace.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if err := add("authority/workspace-binding.json", exportRoleWorkspaceBinding, bindingPath); err != nil {
		return nil, err
	}
	if err := add("authority/events.json", exportRoleEventRoot, filepath.Join(workspace.Store.Directory(), "events.json")); err != nil {
		return nil, err
	}
	segmentDir := filepath.Join(workspace.Store.Directory(), "event-segments")
	entries, err := os.ReadDir(segmentDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, newError(KindRuntime, "STATE_READ_FAILED", "could not read authority event segments for export", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".write-") {
			continue
		}
		if err := add("authority/event-segments/"+entry.Name(), exportRoleEventSegment, filepath.Join(segmentDir, entry.Name())); err != nil {
			return nil, err
		}
	}
	for _, acceptance := range projection.TruthHistory {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if _, err := workspace.Store.ReadTruthFiles(acceptance.Files); err != nil {
			return nil, err
		}
		for _, file := range acceptance.Files {
			name := strings.TrimPrefix(file.Digest, "sha256:")
			if err := add("authority/truth-blobs/"+name, exportRoleTruthBlob, filepath.Join(workspace.Store.Directory(), "truth-blobs", name)); err != nil {
				return nil, err
			}
		}
	}
	for _, changeID := range projection.ChangeOrder {
		for _, evidence := range projection.Evidence[changeID] {
			stdoutSize, stderrSize, err := expectedEvidenceArtifactSizes(projection, evidence)
			if err != nil {
				return nil, err
			}
			if err := workspace.Store.VerifyArtifact(evidence.StdoutArtifact, evidence.StdoutStoredDigest, stdoutSize); err != nil {
				return nil, err
			}
			if err := workspace.Store.VerifyArtifact(evidence.StderrArtifact, evidence.StderrStoredDigest, stderrSize); err != nil {
				return nil, err
			}
			for _, relative := range []string{evidence.StdoutArtifact, evidence.StderrArtifact} {
				normalized, err := normalizePathRoot(relative)
				if err != nil || normalized != relative {
					return nil, newError(KindIntegrity, "ARTIFACT_PATH_INVALID", "Evidence artifact path is not canonical", err)
				}
				if err := add("authority/"+normalized, exportRoleEvidenceArtifact, filepath.Join(workspace.Store.Directory(), filepath.FromSlash(normalized))); err != nil {
					return nil, err
				}
			}
		}
	}
	result := make([]authorityExportSource, 0, len(sources))
	for _, source := range sources {
		result = append(result, source)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].path < result[j].path })
	return result, nil
}

func copyAuthorityExportFile(source authorityExportSource, staging string) (AuthorityExportFile, error) {
	info, err := os.Lstat(source.source)
	if err != nil {
		return AuthorityExportFile{}, newError(KindIntegrity, "EXPORT_SOURCE_MISSING", fmt.Sprintf("authority export source %q is missing", source.path), err)
	}
	limit, err := authorityExportRoleLimit(source.role)
	if err != nil {
		return AuthorityExportFile{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !hasPrivateFilePermissions(info) || info.Size() > limit {
		return AuthorityExportFile{}, newError(KindIntegrity, "EXPORT_SOURCE_UNSAFE", fmt.Sprintf("authority export source %q is not a bounded private regular file", source.path), nil)
	}
	target := filepath.Join(staging, filepath.FromSlash(source.path))
	if err := ensurePrivateDirectory(filepath.Dir(target)); err != nil {
		return AuthorityExportFile{}, err
	}
	input, err := os.Open(source.source)
	if err != nil {
		return AuthorityExportFile{}, newError(KindRuntime, "EXPORT_SOURCE_READ_FAILED", fmt.Sprintf("could not open authority export source %q", source.path), err)
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return AuthorityExportFile{}, newError(KindRuntime, "EXPORT_FILE_CREATE_FAILED", fmt.Sprintf("could not create authority export file %q", source.path), err)
	}
	removeTarget := true
	defer func() {
		_ = output.Close()
		if removeTarget {
			_ = os.Remove(target)
		}
	}()
	written, err := io.Copy(output, io.LimitReader(input, limit+1))
	if err != nil {
		return AuthorityExportFile{}, newError(KindRuntime, "EXPORT_FILE_WRITE_FAILED", fmt.Sprintf("could not copy authority export file %q", source.path), err)
	}
	if written != info.Size() || written > limit {
		return AuthorityExportFile{}, newError(KindIntegrity, "EXPORT_SOURCE_CHANGED", fmt.Sprintf("authority export source %q changed or exceeded its limit while copying", source.path), nil)
	}
	if err := output.Sync(); err != nil {
		return AuthorityExportFile{}, newError(KindRuntime, "EXPORT_FILE_SYNC_FAILED", fmt.Sprintf("could not sync authority export file %q", source.path), err)
	}
	if err := output.Close(); err != nil {
		return AuthorityExportFile{}, newError(KindRuntime, "EXPORT_FILE_CLOSE_FAILED", fmt.Sprintf("could not close authority export file %q", source.path), err)
	}
	removeTarget = false
	digest, err := digestFile(target)
	if err != nil {
		return AuthorityExportFile{}, newError(KindRuntime, "EXPORT_FILE_HASH_FAILED", fmt.Sprintf("could not hash authority export file %q", source.path), err)
	}
	return AuthorityExportFile{Path: source.path, Role: source.role, SizeBytes: written, Digest: digest}, nil
}

func inspectAuthorityExportFiles(ctx context.Context, root string) ([]AuthorityExportFile, int64, error) {
	files := make([]AuthorityExportFile, 0)
	var total int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return newError(KindRuntime, "EXPORT_WALK_FAILED", "could not inspect authority export", walkErr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		info, err := os.Lstat(path)
		if err != nil {
			return newError(KindRuntime, "EXPORT_STAT_FAILED", "could not inspect authority export entry", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return newError(KindIntegrity, "EXPORT_ENTRY_UNSAFE", "authority export cannot contain symlinks", nil)
		}
		if entry.IsDir() {
			if !hasPrivateFilePermissions(info) {
				return newError(KindIntegrity, "EXPORT_ENTRY_UNSAFE", "authority export directories must remain private", nil)
			}
			return nil
		}
		if !info.Mode().IsRegular() || !hasPrivateFilePermissions(info) {
			return newError(KindIntegrity, "EXPORT_ENTRY_UNSAFE", "authority export files must be private regular files", nil)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return newError(KindRuntime, "EXPORT_PATH_FAILED", "could not resolve authority export path", err)
		}
		relative = filepath.ToSlash(relative)
		if relative == "manifest.json" {
			return nil
		}
		role, limit, err := classifyAuthorityExportPath(relative)
		if err != nil {
			return err
		}
		if info.Size() > limit {
			return newError(KindIntegrity, "EXPORT_FILE_TOO_LARGE", fmt.Sprintf("authority export file %q exceeds its role limit", relative), nil)
		}
		if len(files) >= maxAuthorityExportFiles {
			return newError(KindIntegrity, "EXPORT_TOO_MANY_FILES", "authority export exceeds the file-count safety limit", nil)
		}
		digest, err := digestFile(path)
		if err != nil {
			return newError(KindRuntime, "EXPORT_FILE_HASH_FAILED", fmt.Sprintf("could not hash authority export file %q", relative), err)
		}
		total += info.Size()
		if total > maxAuthorityExportBytes {
			return newError(KindIntegrity, "EXPORT_TOO_LARGE", "authority export exceeds the aggregate size limit", nil)
		}
		files = append(files, AuthorityExportFile{Path: relative, Role: role, SizeBytes: info.Size(), Digest: digest})
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, total, nil
}

func classifyAuthorityExportPath(path string) (string, int64, error) {
	normalized, err := normalizePathRoot(path)
	if err != nil || normalized != path || !strings.HasPrefix(path, "authority/") {
		return "", 0, newError(KindIntegrity, "EXPORT_PATH_INVALID", fmt.Sprintf("authority export path %q is invalid", path), err)
	}
	switch path {
	case "authority/workspace-binding.json":
		return exportRoleWorkspaceBinding, maxWorkspaceBindingBytes, nil
	case "authority/events.json":
		return exportRoleEventRoot, maxLegacyEventFileBytes, nil
	}
	if strings.HasPrefix(path, "authority/event-segments/") {
		name := strings.TrimPrefix(path, "authority/event-segments/")
		index := parseCanonicalEventSegmentIndex(name)
		if strings.Contains(name, "/") || index == 0 || name != eventSegmentName(index) {
			return "", 0, newError(KindIntegrity, "EXPORT_PATH_INVALID", fmt.Sprintf("authority export event segment path %q is invalid", path), nil)
		}
		return exportRoleEventSegment, defaultEventSegmentBytes, nil
	}
	if strings.HasPrefix(path, "authority/truth-blobs/") {
		name := strings.TrimPrefix(path, "authority/truth-blobs/")
		if strings.Contains(name, "/") || !isSHA256Digest("sha256:"+name) {
			return "", 0, newError(KindIntegrity, "EXPORT_PATH_INVALID", fmt.Sprintf("authority export truth blob path %q is invalid", path), nil)
		}
		return exportRoleTruthBlob, maxConfigFileBytes, nil
	}
	if strings.HasPrefix(path, "authority/artifacts/") {
		parts := strings.Split(strings.TrimPrefix(path, "authority/artifacts/"), "/")
		if len(parts) != 3 || validateIdentifier(parts[0], "change_id") != nil || validateIdentifier(parts[1], "evidence_id") != nil || (parts[2] != "stdout.log" && parts[2] != "stderr.log") {
			return "", 0, newError(KindIntegrity, "EXPORT_PATH_INVALID", fmt.Sprintf("authority export Evidence artifact path %q is invalid", path), nil)
		}
		return exportRoleEvidenceArtifact, maxEvidenceArtifactBytes, nil
	}
	return "", 0, newError(KindIntegrity, "EXPORT_PATH_INVALID", fmt.Sprintf("authority export path %q has no supported role", path), nil)
}

func parseCanonicalEventSegmentIndex(name string) int {
	if len(name) != len("0000000000000001.json") || !strings.HasSuffix(name, ".json") {
		return 0
	}
	var index int
	for _, character := range strings.TrimSuffix(name, ".json") {
		if character < '0' || character > '9' {
			return 0
		}
		index = index*10 + int(character-'0')
		if index > maxEventSegments {
			return 0
		}
	}
	if index < 1 {
		return 0
	}
	return index
}

func authorityExportRoleLimit(role string) (int64, error) {
	switch role {
	case exportRoleWorkspaceBinding:
		return maxWorkspaceBindingBytes, nil
	case exportRoleEventRoot:
		return maxLegacyEventFileBytes, nil
	case exportRoleEventSegment:
		return defaultEventSegmentBytes, nil
	case exportRoleTruthBlob:
		return maxConfigFileBytes, nil
	case exportRoleEvidenceArtifact:
		return maxEvidenceArtifactBytes, nil
	default:
		return 0, newError(KindIntegrity, "EXPORT_ROLE_INVALID", "authority export file has an unsupported role", nil)
	}
}

func validateAuthorityExportManifest(manifest AuthorityExportManifest) error {
	if manifest.SchemaVersion != SchemaVersion || manifest.Format != authorityExportFormat {
		return newError(KindIntegrity, "EXPORT_MANIFEST_INVALID", "authority export schema or format is unsupported", nil)
	}
	if validateIdentifier(manifest.ProjectID, "project_id") != nil || validateIdentifier(manifest.AuthorityID, "authority_id") != nil || validateIdentifier(manifest.WorkspaceID, "workspace_id") != nil {
		return newError(KindIntegrity, "EXPORT_MANIFEST_INVALID", "authority export identity is invalid", nil)
	}
	if manifest.Revision == 0 || !isSHA256Digest(manifest.EventHead) || !isSHA256Digest(manifest.AcceptedConfigDigest) || !isSHA256Digest(manifest.AcceptedTruthDigest) || !isSHA256Digest(manifest.BundleDigest) {
		return newError(KindIntegrity, "EXPORT_MANIFEST_INVALID", "authority export digests or revision are invalid", nil)
	}
	if manifest.ActivationID != "" && validateIdentifier(manifest.ActivationID, "activation_id") != nil {
		return newError(KindIntegrity, "EXPORT_MANIFEST_INVALID", "authority export activation ID is invalid", nil)
	}
	if len(manifest.Files) == 0 || len(manifest.Files) > maxAuthorityExportFiles {
		return newError(KindIntegrity, "EXPORT_MANIFEST_INVALID", "authority export manifest has an invalid file count", nil)
	}
	previous := ""
	containsEvidence := false
	var total int64
	for _, file := range manifest.Files {
		role, limit, err := classifyAuthorityExportPath(file.Path)
		if err != nil || role != file.Role || file.SizeBytes < 0 || file.SizeBytes > limit || !isSHA256Digest(file.Digest) || (previous != "" && file.Path <= previous) {
			return newError(KindIntegrity, "EXPORT_MANIFEST_INVALID", fmt.Sprintf("authority export file entry %q is invalid", file.Path), err)
		}
		previous = file.Path
		total += file.SizeBytes
		if total > maxAuthorityExportBytes {
			return newError(KindIntegrity, "EXPORT_TOO_LARGE", "authority export manifest exceeds the aggregate size limit", nil)
		}
		containsEvidence = containsEvidence || role == exportRoleEvidenceArtifact
	}
	if containsEvidence != manifest.ContainsEvidenceLogs {
		return newError(KindIntegrity, "EXPORT_MANIFEST_INVALID", "authority export Evidence sensitivity marker is inconsistent", nil)
	}
	return nil
}

func requireExactReferencedExportSet(manifest AuthorityExportManifest, projection Projection) error {
	manifestPaths := make(map[string]string, len(manifest.Files))
	for _, file := range manifest.Files {
		manifestPaths[file.Path] = file.Role
	}
	required := map[string]string{
		"authority/workspace-binding.json": exportRoleWorkspaceBinding,
		"authority/events.json":            exportRoleEventRoot,
	}
	for _, file := range manifest.Files {
		if file.Role == exportRoleEventSegment {
			required[file.Path] = file.Role
		}
	}
	for _, acceptance := range projection.TruthHistory {
		for _, file := range acceptance.Files {
			required["authority/truth-blobs/"+strings.TrimPrefix(file.Digest, "sha256:")] = exportRoleTruthBlob
		}
	}
	for _, changeID := range projection.ChangeOrder {
		for _, evidence := range projection.Evidence[changeID] {
			required["authority/"+evidence.StdoutArtifact] = exportRoleEvidenceArtifact
			required["authority/"+evidence.StderrArtifact] = exportRoleEvidenceArtifact
		}
	}
	if len(required) != len(manifestPaths) {
		return newError(KindIntegrity, "EXPORT_REFERENCE_SET_INVALID", "authority export contains missing or unreferenced state files", nil)
	}
	for path, role := range required {
		if manifestPaths[path] != role {
			return newError(KindIntegrity, "EXPORT_REFERENCE_SET_INVALID", fmt.Sprintf("authority export reference %q is missing or has the wrong role", path), nil)
		}
	}
	return nil
}

func readAuthorityExportManifest(root string) (AuthorityExportManifest, error) {
	path := filepath.Join(root, "manifest.json")
	info, err := os.Lstat(path)
	if err != nil {
		return AuthorityExportManifest{}, newError(KindIntegrity, "EXPORT_MANIFEST_MISSING", "authority export manifest is missing", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !hasPrivateFilePermissions(info) || info.Size() > maxAuthorityExportManifestBytes {
		return AuthorityExportManifest{}, newError(KindIntegrity, "EXPORT_MANIFEST_UNSAFE", "authority export manifest must be a bounded private regular file", nil)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return AuthorityExportManifest{}, newError(KindRuntime, "EXPORT_MANIFEST_READ_FAILED", "could not read authority export manifest", err)
	}
	if len(data) > maxAuthorityExportManifestBytes || !utf8.Valid(data) {
		return AuthorityExportManifest{}, newError(KindIntegrity, "EXPORT_MANIFEST_INVALID", "authority export manifest must be valid UTF-8 JSON", nil)
	}
	var manifest AuthorityExportManifest
	if err := decodeStrictJSON(data, &manifest); err != nil {
		return AuthorityExportManifest{}, newError(KindIntegrity, "EXPORT_MANIFEST_INVALID", "authority export manifest is invalid", err)
	}
	return manifest, nil
}

func readExportedWorkspaceBinding(path, workspaceID string) (WorkspaceBinding, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkspaceBinding{}, newError(KindIntegrity, "EXPORT_BINDING_READ_FAILED", "could not read exported Workspace binding", err)
	}
	var binding WorkspaceBinding
	if err := decodeStrictJSON(data, &binding); err != nil {
		return WorkspaceBinding{}, newError(KindIntegrity, "EXPORT_BINDING_INVALID", "exported Workspace binding is invalid", err)
	}
	if binding.SchemaVersion != SchemaVersion || binding.WorkspaceID != workspaceID || validateIdentifier(binding.ProjectID, "project_id") != nil || validateIdentifier(binding.AuthorityID, "authority_id") != nil || !isSHA256Digest(binding.InitialConfigDigest) || !isSHA256Digest(binding.InitialTruthDigest) || binding.Actor == "" || binding.Reason == "" || binding.Trust == "" || binding.CoreIdentity == "" {
		return WorkspaceBinding{}, newError(KindIntegrity, "EXPORT_BINDING_INVALID", "exported Workspace binding is incomplete or inconsistent", nil)
	}
	return binding, nil
}

func authorityExportManifestDigest(manifest AuthorityExportManifest) (string, error) {
	manifest.BundleDigest = ""
	return digestJSON(manifest)
}

func resolveAuthorityExportTarget(repositoryRoot, stateDir, output string) (string, error) {
	if strings.TrimSpace(output) == "" || strings.ContainsRune(output, '\x00') || !utf8.ValidString(output) {
		return "", newError(KindUsage, "EXPORT_PATH_INVALID", "--output must name a new valid directory", nil)
	}
	target, err := canonicalPotentialPath(output)
	if err != nil {
		return "", err
	}
	if filepath.Base(target) == "." || filepath.Base(target) == string(filepath.Separator) {
		return "", newError(KindUsage, "EXPORT_PATH_INVALID", "--output must name a new directory, not a filesystem root", nil)
	}
	if _, err := os.Lstat(target); err == nil {
		return "", newError(KindConflict, "EXPORT_TARGET_EXISTS", "authority export target already exists", nil)
	} else if !os.IsNotExist(err) {
		return "", newError(KindRuntime, "EXPORT_TARGET_STAT_FAILED", "could not inspect authority export target", err)
	}
	parent, err := canonicalExistingDirectory(filepath.Dir(target))
	if err != nil {
		return "", newError(KindNotFound, "EXPORT_PARENT_NOT_FOUND", "authority export parent directory must already exist", err)
	}
	target = filepath.Join(parent, filepath.Base(target))
	if pathInside(repositoryRoot, target) {
		return "", newError(KindIntegrity, "EXPORT_INSIDE_REPOSITORY", "authority exports must remain outside the Git repository", nil)
	}
	if pathInside(stateDir, target) {
		return "", newError(KindIntegrity, "EXPORT_INSIDE_AUTHORITY", "authority exports must remain outside the live authority state directory", nil)
	}
	return target, nil
}

func openAuthorityExportRoot(path string) (string, error) {
	if strings.TrimSpace(path) == "" || strings.ContainsRune(path, '\x00') || !utf8.ValidString(path) {
		return "", newError(KindUsage, "EXPORT_PATH_INVALID", "--bundle must name a valid authority export directory", nil)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", newError(KindRuntime, "ABS_PATH_FAILED", "could not resolve authority export path", err)
	}
	rawInfo, err := os.Lstat(abs)
	if err != nil {
		return "", newError(KindNotFound, "EXPORT_NOT_FOUND", "authority export directory does not exist", err)
	}
	if rawInfo.Mode()&os.ModeSymlink != 0 || !rawInfo.IsDir() || !hasPrivateFilePermissions(rawInfo) {
		return "", newError(KindIntegrity, "EXPORT_ROOT_UNSAFE", "authority export root must be a private real directory", nil)
	}
	root, err := canonicalExistingDirectory(abs)
	if err != nil {
		return "", err
	}
	return root, nil
}

func sealAuthorityExport(root string) error {
	var directories []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			directories = append(directories, path)
			return nil
		}
		if err := os.Chmod(path, 0o400); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return newError(KindRuntime, "EXPORT_SEAL_FAILED", "could not seal authority export files", err)
	}
	sort.Slice(directories, func(i, j int) bool { return len(directories[i]) > len(directories[j]) })
	for _, directory := range directories {
		if err := os.Chmod(directory, 0o500); err != nil {
			return newError(KindRuntime, "EXPORT_SEAL_FAILED", "could not seal authority export directories", err)
		}
	}
	return nil
}

func removeAuthorityExportTree(root string) {
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			_ = os.Chmod(path, 0o700)
		} else {
			_ = os.Chmod(path, 0o600)
		}
		return nil
	})
	_ = os.RemoveAll(root)
}

func pathInside(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && (relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))))
}

func containsExportRole(files []AuthorityExportFile, role string) bool {
	for _, file := range files {
		if file.Role == role {
			return true
		}
	}
	return false
}

func authorityExportWarning() string {
	return "The bundle contains private project history and may contain sensitive Evidence logs. Its hashes detect accidental or untrusted changes but are not a signature, authenticated approval, remote attestation, or restore operation."
}

func exportResultFromVerification(path string, verified AuthorityVerifyResult) AuthorityExportResult {
	return AuthorityExportResult{
		SchemaVersion:        SchemaVersion,
		Path:                 path,
		BundleDigest:         verified.BundleDigest,
		ProjectID:            verified.ProjectID,
		AuthorityID:          verified.AuthorityID,
		WorkspaceID:          verified.WorkspaceID,
		Revision:             verified.Revision,
		EventHead:            verified.EventHead,
		FileCount:            verified.FileCount,
		TotalBytes:           verified.TotalBytes,
		ContainsEvidenceLogs: verified.ContainsEvidenceLogs,
		Trust:                verified.Trust,
		Warning:              verified.Warning,
	}
}
