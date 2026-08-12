package ecp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAuthorityHealthIsAuthorityOnlyExactAndHealthy(t *testing.T) {
	ctx := context.Background()
	service, repo := createAuthorityExportProject(t, false)
	_, before, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".ecp", "project.json"), []byte("{malformed"), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := service.AuthorityHealth(ctx, repo)
	if err != nil {
		t.Fatalf("authority-only health was blocked by malformed candidate config: %v", err)
	}
	if report.Status != AuthorityHealthHealthy || !report.GateLeaseAcquired || report.FindingCount != 0 || report.ProjectID != before.ProjectID || report.AuthorityID == "" || report.WorkspaceID != before.WorkspaceID {
		t.Fatalf("healthy authority report was incomplete: %+v", report)
	}
	if report.Events.Revision != before.Projection.Revision || report.Events.EventHead != before.Projection.EventHead || report.Events.TotalSegments != 1 || report.Events.TotalBytes == 0 || report.Events.RemainingMutationBytes <= 0 {
		t.Fatalf("event capacity snapshot was not exact: %+v", report.Events)
	}
	if report.TruthBlobs.ReferencedFiles == 0 || report.TruthBlobs.VerifiedFiles != report.TruthBlobs.ReferencedFiles || report.TruthBlobs.InvalidReferencedFiles != 0 {
		t.Fatalf("accepted Project Truth objects were not completely verified: %+v", report.TruthBlobs)
	}

	projection, err := before.Store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Revision != before.Projection.Revision || projection.EventHead != before.Projection.EventHead {
		t.Fatalf("logical read-only health mutated authority history: before=%+v after=%+v", before.Projection, projection)
	}
}

func TestAuthorityHealthReportsOrphansAndTemporaryRemnantsWithoutDeletingThem(t *testing.T) {
	ctx := context.Background()
	service, repo := createAuthorityExportProject(t, false)
	_, workspace, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	orphanTruth := []byte("unreferenced Project Truth object\n")
	orphanDigest := digestBytes(orphanTruth)
	if err := workspace.Store.WriteTruthBlobs([]TruthFileContent{{Path: "contracts/orphan.md", Digest: orphanDigest, Content: orphanTruth}}); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Store.WriteArtifacts("chg-orphan-health", "evd-orphan-health", []byte("orphan stdout"), []byte("orphan stderr")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace.Store.Directory(), ".write-abandoned"), []byte("event temp"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace.Store.Directory(), "truth-blobs", ".write-abandoned"), []byte("truth temp"), 0o600); err != nil {
		t.Fatal(err)
	}
	artifactTemp := filepath.Join(workspace.Store.Directory(), "artifacts", "chg-orphan-health", ".artifact-abandoned")
	if err := os.Mkdir(artifactTemp, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactTemp, "stdout.log"), []byte("partial temp"), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := service.AuthorityHealth(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != AuthorityHealthAttention || report.FindingCount == 0 {
		t.Fatalf("orphan and temporary state did not require attention: %+v", report)
	}
	if report.TruthBlobs.OrphanFiles != 1 || report.TruthBlobs.TemporaryEntries != 1 {
		t.Fatalf("Project Truth orphan inventory was inaccurate: %+v", report.TruthBlobs)
	}
	if report.EvidenceArtifacts.OrphanFiles != 2 || report.EvidenceArtifacts.TemporaryEntries != 1 || report.EvidenceArtifacts.TemporaryBytes == 0 {
		t.Fatalf("Evidence orphan inventory was inaccurate: %+v", report.EvidenceArtifacts)
	}
	if report.Events.TemporaryFiles != 1 || report.Events.TemporaryBytes == 0 {
		t.Fatalf("event write remnant was not inventoried: %+v", report.Events)
	}
	for _, path := range []string{
		filepath.Join(workspace.Store.Directory(), "truth-blobs", stringsTrimSHA256(orphanDigest)),
		filepath.Join(workspace.Store.Directory(), "artifacts", "chg-orphan-health", "evd-orphan-health", "stdout.log"),
		filepath.Join(workspace.Store.Directory(), ".write-abandoned"),
		artifactTemp,
	} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("health deleted or repaired inventoried state %s: %v", path, err)
		}
	}
}

func TestAuthorityHealthReturnsIndeterminateReportForCorruptReferences(t *testing.T) {
	ctx := context.Background()
	service, repo := createAuthorityExportProject(t, true)
	_, workspace, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	evidence := workspace.Projection.Evidence[workspace.Projection.ChangeOrder[len(workspace.Projection.ChangeOrder)-1]][0]
	artifact := filepath.Join(workspace.Store.Directory(), filepath.FromSlash(evidence.StdoutArtifact))
	if err := os.WriteFile(artifact, []byte("tampered Evidence output"), 0o600); err != nil {
		t.Fatal(err)
	}
	truthDigest := workspace.Projection.TruthHistory[0].Files[0].Digest
	truthPath := filepath.Join(workspace.Store.Directory(), "truth-blobs", stringsTrimSHA256(truthDigest))
	if err := os.Remove(truthPath); err != nil {
		t.Fatal(err)
	}

	report, err := service.AuthorityHealth(ctx, repo)
	if err != nil {
		t.Fatalf("object corruption should be represented by a structured health report: %v", err)
	}
	if report.Status != AuthorityHealthIndeterminate || report.TruthBlobs.InvalidReferencedFiles == 0 || report.EvidenceArtifacts.InvalidReferencedFiles == 0 {
		t.Fatalf("corrupt referenced objects did not make health indeterminate: %+v", report)
	}
	if !healthHasFinding(report, "TRUTH_BLOB_MISSING") || !healthHasFinding(report, "ARTIFACT_DIGEST_MISMATCH") {
		t.Fatalf("health report lost exact corruption diagnostics: %+v", report.Findings)
	}
}

func TestAuthorityHealthRejectsUntrustedEventProjectionAndReportsUnsafeOrphan(t *testing.T) {
	ctx := context.Background()
	service, repo := createAuthorityExportProject(t, false)
	_, workspace, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	unsafe := filepath.Join(workspace.Store.Directory(), "truth-blobs", "unsafe-link")
	if err := os.Symlink(filepath.Join(workspace.Store.Directory(), "events.json"), unsafe); err != nil {
		t.Fatal(err)
	}
	report, err := service.AuthorityHealth(ctx, repo)
	if err != nil || report.Status != AuthorityHealthIndeterminate || !healthHasFinding(report, "TRUTH_BLOB_ENTRY_UNSAFE") {
		t.Fatalf("unsafe unreferenced state was not disclosed without being trusted: report=%+v err=%v", report, err)
	}
	if err := os.Remove(unsafe); err != nil {
		t.Fatal(err)
	}

	eventsPath := filepath.Join(workspace.Store.Directory(), "events.json")
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := bytes.Replace(data, []byte(`"origin": "cli"`), []byte(`"origin": "clx"`), 1)
	if bytes.Equal(data, tampered) {
		t.Fatal("could not locate an event origin to tamper")
	}
	if err := os.WriteFile(eventsPath, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if report, err := service.AuthorityHealth(ctx, repo); err == nil || report.Status != "" || !isErrorCode(err, "EVENT_HASH_INVALID") {
		t.Fatalf("health constructed a report from an untrusted event projection: report=%+v err=%v", report, err)
	}
}

func TestAuthorityHealthDoesNotRepairUnsafeAuthorityDirectory(t *testing.T) {
	ctx := context.Background()
	service, repo := createAuthorityExportProject(t, false)
	_, workspace, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	stateDir := workspace.Store.Directory()
	if err := os.Chmod(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(stateDir, 0o700) })

	if report, err := service.AuthorityHealth(ctx, repo); err == nil || report.Status != "" || !isErrorCode(err, "AUTHORITY_STATE_DIR_UNSAFE") {
		t.Fatalf("health trusted or repaired an unsafe authority directory: report=%+v err=%v", report, err)
	}
	info, err := os.Lstat(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("health changed authority directory permissions: got %04o want 0755", info.Mode().Perm())
	}
}

func TestAuthorityHealthDoesNotFollowObjectStoreSymlinks(t *testing.T) {
	ctx := context.Background()
	service, repo := createAuthorityExportProject(t, true)
	_, workspace, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}

	truthRoot := filepath.Join(workspace.Store.Directory(), "truth-blobs")
	movedTruthRoot := filepath.Join(workspace.Store.Directory(), "truth-blobs-moved")
	if err := os.Rename(truthRoot, movedTruthRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(movedTruthRoot, truthRoot); err != nil {
		t.Fatal(err)
	}

	changeID := workspace.Projection.ChangeOrder[len(workspace.Projection.ChangeOrder)-1]
	changeRoot := filepath.Join(workspace.Store.Directory(), "artifacts", changeID)
	movedChangeRoot := filepath.Join(workspace.Store.Directory(), "artifacts-moved")
	if err := os.Rename(changeRoot, movedChangeRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(movedChangeRoot, changeRoot); err != nil {
		t.Fatal(err)
	}

	report, err := service.AuthorityHealth(ctx, repo)
	if err != nil {
		t.Fatalf("unsafe object stores should produce a structured report after the event projection is trusted: %v", err)
	}
	if report.Status != AuthorityHealthIndeterminate || report.TruthBlobs.VerifiedFiles != 0 || report.TruthBlobs.InvalidReferencedFiles != report.TruthBlobs.ReferencedFiles {
		t.Fatalf("health followed or trusted the Project Truth store symlink: %+v", report)
	}
	if report.EvidenceArtifacts.VerifiedFiles != 0 || report.EvidenceArtifacts.InvalidReferencedFiles != report.EvidenceArtifacts.ReferencedFiles || !healthHasFinding(report, "ARTIFACT_UNSAFE") {
		t.Fatalf("health followed or trusted the Evidence ancestor symlink: %+v", report)
	}
	if !healthHasFinding(report, "TRUTH_BLOB_STORE_UNSAFE") {
		t.Fatalf("unsafe Project Truth store was not disclosed: %+v", report.Findings)
	}
}

func TestAuthorityHealthExposesButDoesNotRecoverReleasedGateRun(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Health GateRun", Goal: "Diagnose an abandoned run without mutating it", Scope: []string{"src"},
		AcceptanceCriteria: []string{"health exposes the exact open run"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "health GateRun\n")
	if err := assessPreservedTestChange(t, ctx, service, repo, change.ChangeID); err != nil {
		t.Fatal(err)
	}
	plan, err := service.PlanGates(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	openRun := appendOpenGateRun(t, ctx, service, repo, change, plan)
	_, before, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}

	report, err := service.AuthorityHealth(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != AuthorityHealthAttention || report.ActiveGateRun == nil || report.ActiveGateRun.RunID != openRun.RunID || !healthHasFinding(report, "GATE_RUN_INTERRUPTION_RECOVERABLE") {
		t.Fatalf("released open GateRun was not diagnosed precisely: %+v", report)
	}
	after, err := before.Store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Projection.Revision || after.ActiveGateRun() == nil || after.ActiveGateRun().State != GateRunInProgress {
		t.Fatalf("health silently recovered or mutated the open GateRun: before=%+v after=%+v", before.Projection.ActiveGateRun(), after.ActiveGateRun())
	}
}

func TestAuthorityHealthCannotMisclassifyLiveGateLeaseOwner(t *testing.T) {
	ctx := context.Background()
	service, repo := createAuthorityExportProject(t, false)
	_, workspace, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	release, err := workspace.Store.AcquireGateLease(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	limited, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	if _, err := service.AuthorityHealth(limited, repo); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("health guessed around a live Gate lease instead of waiting/failing closed: %v", err)
	}
}

func TestAuthorityHealthReportsSegmentedCapacityAndThresholds(t *testing.T) {
	ctx := context.Background()
	service, repo := createAuthorityExportProject(t, false)
	forceAuthorityEventSegments(t, service, repo, 2)
	report, err := service.AuthorityHealth(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if report.Events.ContinuationSegments < 2 || report.Events.TotalSegments != report.Events.ContinuationSegments+1 || report.Events.RemainingContinuationSegments != maxEventSegments-report.Events.ContinuationSegments || report.Events.TotalBytes <= report.Events.CurrentSegmentBytes {
		t.Fatalf("segmented authority capacity was not reported exactly: %+v", report.Events)
	}

	projection := NewProjection()
	projection.Activation = &ProjectActivation{Enabled: true}
	limit := eventStoreLimit(projection)
	nearBytes := authorityEventHealthFromHistory(eventHistory{projection: projection, totalBytes: (limit*80 + 99) / 100}, defaultEventSegmentBytes)
	if !nearBytes.NearCapacity {
		t.Fatalf("80 percent event byte utilization did not trigger the stable warning threshold: %+v", nearBytes)
	}
	nearSegments := authorityEventHealthFromHistory(eventHistory{projection: projection, segmentCount: maxEventSegments - 100}, defaultEventSegmentBytes)
	if !nearSegments.NearCapacity {
		t.Fatalf("low remaining continuation capacity did not trigger the warning threshold: %+v", nearSegments)
	}
}

func healthHasFinding(report AuthorityHealthReport, code string) bool {
	for _, finding := range report.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

func stringsTrimSHA256(digest string) string {
	const prefix = "sha256:"
	if len(digest) >= len(prefix) && digest[:len(prefix)] == prefix {
		return digest[len(prefix):]
	}
	return digest
}
