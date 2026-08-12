package ecp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAuthorityExportIsDeterministicPrivateAndOfflineVerifiable(t *testing.T) {
	ctx := context.Background()
	service, repo := createAuthorityExportProject(t, true)
	_, before, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}

	firstPath := filepath.Join(t.TempDir(), "first-export")
	first, err := service.ExportAuthority(ctx, repo, firstPath)
	if err != nil {
		t.Fatalf("authority export failed: %v", err)
	}
	registerAuthorityExportCleanup(t, first.Path)
	canonicalFirst, err := canonicalExistingDirectory(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if first.Path != canonicalFirst || first.BundleDigest == "" || first.Revision != before.Projection.Revision || first.EventHead != before.Projection.EventHead || !first.ContainsEvidenceLogs || first.FileCount == 0 || first.TotalBytes == 0 {
		t.Fatalf("unexpected authority export result: %+v", first)
	}
	rootInfo, err := os.Stat(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if rootInfo.Mode().Perm()&0o222 != 0 || rootInfo.Mode().Perm()&0o077 != 0 {
		t.Fatalf("authority export root was not sealed private: mode=%o", rootInfo.Mode().Perm())
	}
	verified, err := VerifyAuthorityExport(ctx, firstPath)
	if err != nil || !verified.Valid || verified.BundleDigest != first.BundleDigest || verified.Revision != first.Revision || verified.EventHead != first.EventHead {
		t.Fatalf("offline authority verification failed: %+v err=%v", verified, err)
	}

	secondPath := filepath.Join(t.TempDir(), "second-export")
	second, err := service.ExportAuthority(ctx, repo, secondPath)
	if err != nil {
		t.Fatal(err)
	}
	registerAuthorityExportCleanup(t, second.Path)
	if second.BundleDigest != first.BundleDigest || second.FileCount != first.FileCount || second.TotalBytes != first.TotalBytes {
		t.Fatalf("unchanged authority did not produce deterministic exports: first=%+v second=%+v", first, second)
	}
	_, after, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if after.Projection.Revision != before.Projection.Revision || after.Projection.EventHead != before.Projection.EventHead {
		t.Fatalf("read-only export mutated live authority: before=%+v after=%+v", before.Projection, after.Projection)
	}
}

func TestAuthorityExportIgnoresOrphansAndSurvivesMalformedCandidate(t *testing.T) {
	ctx := context.Background()
	service, repo := createAuthorityExportProject(t, false)
	_, workspace, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Store.WriteArtifacts("chg-orphan0001", "evd-orphan0001", []byte("orphan stdout"), []byte("orphan stderr")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".ecp", "project.json"), []byte("{malformed"), 0o600); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(t.TempDir(), "authority-only-export")
	result, err := service.ExportAuthority(ctx, repo, output)
	if err != nil {
		t.Fatalf("malformed candidate blocked authority-only export: %v", err)
	}
	registerAuthorityExportCleanup(t, result.Path)
	manifest, err := readAuthorityExportManifest(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range manifest.Files {
		if bytes.Contains([]byte(file.Path), []byte("orphan")) {
			t.Fatalf("unreferenced artifact leaked into export: %+v", file)
		}
	}
	if result.ContainsEvidenceLogs {
		t.Fatalf("orphan Evidence files incorrectly set sensitivity marker: %+v", result)
	}
	if _, err := VerifyAuthorityExport(ctx, output); err != nil {
		t.Fatalf("authority-only export did not verify: %v", err)
	}
}

func TestAuthorityExportVerificationDetectsFileTampering(t *testing.T) {
	ctx := context.Background()
	service, repo := createAuthorityExportProject(t, false)
	output := filepath.Join(t.TempDir(), "tamper-export")
	result, err := service.ExportAuthority(ctx, repo, output)
	if err != nil {
		t.Fatal(err)
	}
	registerAuthorityExportCleanup(t, result.Path)
	makeAuthorityExportWritable(t, output)
	manifest, err := readAuthorityExportManifest(output)
	if err != nil {
		t.Fatal(err)
	}
	var truthPath string
	for _, file := range manifest.Files {
		if file.Role == exportRoleTruthBlob {
			truthPath = filepath.Join(output, filepath.FromSlash(file.Path))
			break
		}
	}
	if truthPath == "" {
		t.Fatal("export did not contain an accepted Project Truth blob")
	}
	content, err := os.ReadFile(truthPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) == 0 {
		t.Fatal("test truth blob is empty")
	}
	content[0] ^= 1
	if err := os.WriteFile(truthPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyAuthorityExport(ctx, output); err == nil || !isErrorCode(err, "EXPORT_FILE_DIGEST_MISMATCH") {
		t.Fatalf("export file tampering was not caught by manifest verification: %v", err)
	}
}

func TestAuthorityExportEventChainSurvivesRecomputedManifestAttack(t *testing.T) {
	ctx := context.Background()
	service, repo := createAuthorityExportProject(t, false)
	output := filepath.Join(t.TempDir(), "semantic-tamper-export")
	result, err := service.ExportAuthority(ctx, repo, output)
	if err != nil {
		t.Fatal(err)
	}
	registerAuthorityExportCleanup(t, result.Path)
	makeAuthorityExportWritable(t, output)
	eventsPath := filepath.Join(output, "authority", "events.json")
	events, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := bytes.Replace(events, []byte(`"origin": "cli"`), []byte(`"origin": "clx"`), 1)
	if bytes.Equal(events, tampered) {
		t.Fatal("test could not locate an event origin to tamper")
	}
	if err := os.WriteFile(eventsPath, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := readAuthorityExportManifest(output)
	if err != nil {
		t.Fatal(err)
	}
	for index := range manifest.Files {
		if manifest.Files[index].Path == "authority/events.json" {
			manifest.Files[index].Digest = digestBytes(tampered)
			manifest.Files[index].SizeBytes = int64(len(tampered))
		}
	}
	manifest.BundleDigest, err = authorityExportManifestDigest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeAuthorityExportManifestForTest(t, output, manifest)
	if _, err := VerifyAuthorityExport(ctx, output); err == nil || !isErrorCode(err, "EVENT_HASH_INVALID") {
		t.Fatalf("recomputed outer manifest bypassed the authority event chain: %v", err)
	}
}

func TestAuthorityExportRejectsUnsafeTargetsAndBundleRoots(t *testing.T) {
	ctx := context.Background()
	service, repo := createAuthorityExportProject(t, false)
	if _, err := service.ExportAuthority(ctx, repo, filepath.Join(repo, "export")); err == nil || !isErrorCode(err, "EXPORT_INSIDE_REPOSITORY") {
		t.Fatalf("export inside repository was not rejected: %v", err)
	}
	if _, err := service.ExportAuthority(ctx, repo, filepath.Join(service.StateDir, "export")); err == nil || !isErrorCode(err, "EXPORT_INSIDE_AUTHORITY") {
		t.Fatalf("export inside live authority was not rejected: %v", err)
	}
	existing := t.TempDir()
	if _, err := service.ExportAuthority(ctx, repo, existing); err == nil || !isErrorCode(err, "EXPORT_TARGET_EXISTS") {
		t.Fatalf("existing export target was not rejected: %v", err)
	}

	output := filepath.Join(t.TempDir(), "real-export")
	result, err := service.ExportAuthority(ctx, repo, output)
	if err != nil {
		t.Fatal(err)
	}
	registerAuthorityExportCleanup(t, result.Path)
	link := filepath.Join(t.TempDir(), "export-link")
	if err := os.Symlink(output, link); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyAuthorityExport(ctx, link); err == nil || !isErrorCode(err, "EXPORT_ROOT_UNSAFE") {
		t.Fatalf("symlink export root was not rejected: %v", err)
	}
}

func TestAuthorityExportPreservesSegmentedHistory(t *testing.T) {
	ctx := context.Background()
	service, repo := createAuthorityExportProject(t, false)
	forceAuthorityEventSegments(t, service, repo, 2)
	output := filepath.Join(t.TempDir(), "segmented-export")
	result, err := service.ExportAuthority(ctx, repo, output)
	if err != nil {
		t.Fatal(err)
	}
	registerAuthorityExportCleanup(t, result.Path)
	manifest, err := readAuthorityExportManifest(output)
	if err != nil {
		t.Fatal(err)
	}
	segments := 0
	for _, file := range manifest.Files {
		if file.Role == exportRoleEventSegment {
			segments++
		}
	}
	if segments < 2 {
		t.Fatalf("segmented authority export omitted continuation history: %+v", manifest.Files)
	}
	verified, err := VerifyAuthorityExport(ctx, output)
	if err != nil || verified.Revision != result.Revision || verified.EventHead != result.EventHead {
		t.Fatalf("segmented authority export did not replay offline: %+v err=%v", verified, err)
	}
}

func TestAuthorityExportVerificationRejectsMissingAndExtraFiles(t *testing.T) {
	ctx := context.Background()
	service, repo := createAuthorityExportProject(t, false)

	missingOutput := filepath.Join(t.TempDir(), "missing-file-export")
	missing, err := service.ExportAuthority(ctx, repo, missingOutput)
	if err != nil {
		t.Fatal(err)
	}
	registerAuthorityExportCleanup(t, missing.Path)
	makeAuthorityExportWritable(t, missingOutput)
	manifest, err := readAuthorityExportManifest(missingOutput)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(missingOutput, filepath.FromSlash(manifest.Files[len(manifest.Files)-1].Path))); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyAuthorityExport(ctx, missingOutput); err == nil || !isErrorCode(err, "EXPORT_LAYOUT_INVALID") {
		t.Fatalf("missing manifest file was not rejected: %v", err)
	}

	extraOutput := filepath.Join(t.TempDir(), "extra-file-export")
	extra, err := service.ExportAuthority(ctx, repo, extraOutput)
	if err != nil {
		t.Fatal(err)
	}
	registerAuthorityExportCleanup(t, extra.Path)
	makeAuthorityExportWritable(t, extraOutput)
	if err := os.WriteFile(filepath.Join(extraOutput, "authority", "unreferenced.txt"), []byte("not part of authority"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyAuthorityExport(ctx, extraOutput); err == nil || !isErrorCode(err, "EXPORT_PATH_INVALID") {
		t.Fatalf("extra unreferenced export file was not rejected: %v", err)
	}
}

func createAuthorityExportProject(t *testing.T, withEvidence bool) (Service, string) {
	t.Helper()
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	if !withEvidence {
		return service, repo
	}
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title:              "Export Evidence",
		Goal:               "Preserve one referenced Evidence record in the portable authority bundle",
		Scope:              []string{"src"},
		AcceptanceCriteria: []string{"the authority export verifies offline"},
		Risk:               RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "authority export evidence\n")
	result, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil)
	if err != nil || len(result.Evidence) == 0 {
		t.Fatalf("could not create export Evidence fixture: %+v err=%v", result, err)
	}
	return service, repo
}

func forceAuthorityEventSegments(t *testing.T, service Service, repo string, minimumSegments int) {
	t.Helper()
	ctx := context.Background()
	config, err := LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, err := service.Git.WorkspaceID(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(service.StateDir, config.Project.ProjectID, workspaceID, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	legacyInfo, err := os.Stat(filepath.Join(store.Directory(), "events.json"))
	if err != nil {
		t.Fatal(err)
	}
	store.eventSegmentBytes = int(legacyInfo.Size())
	for attempt := 1; attempt <= 200; attempt++ {
		acceptance := *projection.AcceptedConfig
		acceptance.Reason = fmt.Sprintf("authority export segment %03d", attempt)
		revision := projection.Revision
		projection, err = store.Append(ctx, &revision, PendingEvent{Type: "config_accepted", Origin: "test", Payload: acceptance})
		if err != nil {
			t.Fatal(err)
		}
		entries, readErr := os.ReadDir(filepath.Join(store.Directory(), "event-segments"))
		if readErr == nil && len(entries) >= minimumSegments {
			return
		}
		if readErr != nil && !os.IsNotExist(readErr) {
			t.Fatal(readErr)
		}
	}
	t.Fatalf("could not force %d authority event segments", minimumSegments)
}

func makeAuthorityExportWritable(t *testing.T, root string) {
	t.Helper()
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o700)
		}
		return os.Chmod(path, 0o600)
	}); err != nil {
		t.Fatal(err)
	}
}

func registerAuthorityExportCleanup(t *testing.T, root string) {
	t.Helper()
	t.Cleanup(func() {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
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
	})
}

func writeAuthorityExportManifestForTest(t *testing.T, root string, manifest AuthorityExportManifest) {
	t.Helper()
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
