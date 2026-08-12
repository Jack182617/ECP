package ecp

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxAuthorityHealthFindings = 100

type AuthorityHealthStatus string

const (
	AuthorityHealthHealthy       AuthorityHealthStatus = "HEALTHY"
	AuthorityHealthAttention     AuthorityHealthStatus = "ATTENTION"
	AuthorityHealthIndeterminate AuthorityHealthStatus = "INDETERMINATE"
)

type AuthorityHealthSeverity string

const (
	AuthorityHealthSeverityAttention     AuthorityHealthSeverity = "ATTENTION"
	AuthorityHealthSeverityIndeterminate AuthorityHealthSeverity = "INDETERMINATE"
)

type AuthorityHealthFinding struct {
	Severity AuthorityHealthSeverity `json:"severity"`
	Code     string                  `json:"code"`
	Message  string                  `json:"message"`
	Path     string                  `json:"path,omitempty"`
}

type AuthorityEventHealth struct {
	Revision                      uint64 `json:"revision"`
	EventHead                     string `json:"event_head"`
	TotalSegments                 int    `json:"total_segments"`
	ContinuationSegments          int    `json:"continuation_segments"`
	MaximumContinuationSegments   int    `json:"maximum_continuation_segments"`
	RemainingContinuationSegments int    `json:"remaining_continuation_segments"`
	CurrentSegmentIndex           int    `json:"current_segment_index"`
	CurrentSegmentBytes           int64  `json:"current_segment_bytes"`
	RotationTargetBytes           int64  `json:"rotation_target_bytes"`
	TotalBytes                    int64  `json:"total_bytes"`
	HardLimitBytes                int64  `json:"hard_limit_bytes"`
	CurrentMutationLimitBytes     int64  `json:"current_mutation_limit_bytes"`
	RemainingMutationBytes        int64  `json:"remaining_mutation_bytes"`
	TerminalReserveBytes          int64  `json:"terminal_reserve_bytes"`
	TemporaryFiles                int    `json:"temporary_files"`
	TemporaryBytes                int64  `json:"temporary_bytes"`
	UnrecognizedEntries           int    `json:"unrecognized_entries"`
	UnsafeEntries                 int    `json:"unsafe_entries"`
	NearCapacity                  bool   `json:"near_capacity"`
}

type AuthorityObjectHealth struct {
	ReferencedFiles        int   `json:"referenced_files"`
	VerifiedFiles          int   `json:"verified_files"`
	VerifiedBytes          int64 `json:"verified_bytes"`
	InvalidReferencedFiles int   `json:"invalid_referenced_files"`
	OrphanFiles            int   `json:"orphan_files"`
	OrphanBytes            int64 `json:"orphan_bytes"`
	OrphanDirectories      int   `json:"orphan_directories"`
	TemporaryEntries       int   `json:"temporary_entries"`
	TemporaryBytes         int64 `json:"temporary_bytes"`
	UnrecognizedEntries    int   `json:"unrecognized_entries"`
	UnsafeEntries          int   `json:"unsafe_entries"`
}

type AuthorityHealthReport struct {
	SchemaVersion     int                      `json:"schema_version"`
	Status            AuthorityHealthStatus    `json:"status"`
	Root              string                   `json:"root"`
	ProjectID         string                   `json:"project_id"`
	AuthorityID       string                   `json:"authority_id"`
	WorkspaceID       string                   `json:"workspace_id"`
	Enabled           bool                     `json:"enabled"`
	ActivationID      string                   `json:"activation_id,omitempty"`
	ActiveChangeID    string                   `json:"active_change_id,omitempty"`
	ActiveGateRun     *GateRun                 `json:"active_gate_run,omitempty"`
	GateLeaseAcquired bool                     `json:"gate_lease_acquired"`
	Events            AuthorityEventHealth     `json:"events"`
	TruthBlobs        AuthorityObjectHealth    `json:"truth_blobs"`
	EvidenceArtifacts AuthorityObjectHealth    `json:"evidence_artifacts"`
	FindingCount      int                      `json:"finding_count"`
	FindingsTruncated int                      `json:"findings_truncated"`
	Findings          []AuthorityHealthFinding `json:"findings"`
	ObservedAt        time.Time                `json:"observed_at"`
	Trust             string                   `json:"trust"`
	Warning           string                   `json:"warning"`
}

type authorityHealthBuilder struct {
	report *AuthorityHealthReport
}

func (builder *authorityHealthBuilder) add(severity AuthorityHealthSeverity, code, message, path string) {
	builder.report.FindingCount++
	if severity == AuthorityHealthSeverityIndeterminate {
		builder.report.Status = AuthorityHealthIndeterminate
	} else if builder.report.Status == AuthorityHealthHealthy {
		builder.report.Status = AuthorityHealthAttention
	}
	if len(builder.report.Findings) >= maxAuthorityHealthFindings {
		builder.report.FindingsTruncated++
		return
	}
	builder.report.Findings = append(builder.report.Findings, AuthorityHealthFinding{
		Severity: severity,
		Code:     code,
		Message:  message,
		Path:     path,
	})
}

type authorityHealthBudget struct {
	entries int
	bytes   int64
}

func (budget *authorityHealthBudget) consume(info os.FileInfo) error {
	budget.entries++
	if budget.entries > maxAuthorityExportFiles {
		return newError(KindIntegrity, "AUTHORITY_HEALTH_TOO_MANY_ENTRIES", "authority health inventory exceeds the supported entry-count limit", nil)
	}
	if info.Mode().IsRegular() {
		budget.bytes += info.Size()
		if budget.bytes > maxAuthorityExportBytes {
			return newError(KindIntegrity, "AUTHORITY_HEALTH_TOO_LARGE", "authority health inventory exceeds the supported aggregate size", nil)
		}
	}
	return nil
}

// AuthorityHealth constructs an exact, authority-only inventory. It acquires
// the Gate lease before the mutation lock, matching lifecycle lock order. The
// operation never appends an event, repairs an object, deletes an orphan, or
// rewrites repository files. Advisory lock metadata may be refreshed.
func (s Service) AuthorityHealth(ctx context.Context, start string) (AuthorityHealthReport, error) {
	service, workspace, err := s.resolveAuthorityTarget(ctx, start)
	if err != nil {
		return AuthorityHealthReport{}, err
	}
	releaseGate, err := workspace.Store.acquireExistingGateLease(ctx)
	if err != nil {
		return AuthorityHealthReport{}, err
	}
	defer releaseGate()
	releaseMutation, err := workspace.Store.acquireExistingLock(ctx)
	if err != nil {
		return AuthorityHealthReport{}, err
	}
	defer releaseMutation()

	binding, err := loadWorkspaceBinding(service.StateDir, workspace.WorkspaceID)
	if err != nil {
		return AuthorityHealthReport{}, err
	}
	if binding != workspace.Binding {
		return AuthorityHealthReport{}, newError(KindIntegrity, "WORKSPACE_BINDING_CHANGED", "Workspace binding changed while authority health was acquiring its snapshot", nil)
	}
	history, err := workspace.Store.loadEventHistory(ctx, false)
	if err != nil {
		return AuthorityHealthReport{}, err
	}
	projection := history.projection
	if projection.Registration == nil || projection.Registration.AuthorityID != service.authorityID || projection.Registration.ProjectID != workspace.ProjectID || projection.Registration.WorkspaceID != workspace.WorkspaceID {
		return AuthorityHealthReport{}, newError(KindIntegrity, "AUTHORITY_REGISTRATION_MISMATCH", "authority history belongs to a different authority-state location or Workspace", nil)
	}

	report := AuthorityHealthReport{
		SchemaVersion:     SchemaVersion,
		Status:            AuthorityHealthHealthy,
		Root:              workspace.Root,
		ProjectID:         workspace.ProjectID,
		AuthorityID:       service.authorityID,
		WorkspaceID:       workspace.WorkspaceID,
		Enabled:           projection.ECPEnabled(),
		GateLeaseAcquired: true,
		Findings:          []AuthorityHealthFinding{},
		ObservedAt:        service.Clock().UTC(),
		Trust:             "local-authority-consistent-snapshot",
		Warning:           "This logical read-only report may refresh private advisory-lock metadata. It does not repair, delete, restore, compact, migrate, or attest the authority, and it does not inspect candidate .ecp files.",
	}
	if projection.Activation != nil {
		report.ActivationID = projection.Activation.ActivationID
	}
	if change := projection.ActiveChange(); change != nil {
		report.ActiveChangeID = change.ChangeID
	}
	if run := projection.ActiveGateRun(); run != nil {
		report.ActiveGateRun = run
	}
	report.Events = authorityEventHealthFromHistory(history, workspace.Store.eventSegmentLimit())
	builder := authorityHealthBuilder{report: &report}
	if report.Events.NearCapacity {
		builder.add(AuthorityHealthSeverityAttention, "EVENT_CAPACITY_NEAR_LIMIT", "authority event history is near its current byte or continuation-segment mutation limit", "events.json")
	}
	if report.ActiveGateRun != nil {
		builder.add(AuthorityHealthSeverityAttention, "GATE_RUN_INTERRUPTION_RECOVERABLE", "an IN_PROGRESS GateRun remains after this diagnostic acquired the released Gate lease; health does not mutate it, and the next lifecycle operation may record it as INTERRUPTED", "")
	}

	scanBudget := &authorityHealthBudget{}
	if err := scanAuthorityEventLayout(ctx, workspace.Store, &report.Events, &builder, scanBudget); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return AuthorityHealthReport{}, err
		}
		addHealthError(&builder, err, "")
	}

	truthExpected := verifyReferencedTruthBlobs(ctx, workspace.Store, projection, &report.TruthBlobs, &builder)
	if err := ctx.Err(); err != nil {
		return AuthorityHealthReport{}, err
	}
	if err := scanTruthBlobStore(ctx, workspace.Store, truthExpected, &report.TruthBlobs, &builder, scanBudget); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return AuthorityHealthReport{}, err
		}
		addHealthError(&builder, err, "truth-blobs")
	}

	evidenceExpected, evidenceDirectories := verifyReferencedEvidence(ctx, workspace.Store, projection, &report.EvidenceArtifacts, &builder)
	if err := ctx.Err(); err != nil {
		return AuthorityHealthReport{}, err
	}
	if err := scanEvidenceArtifactStore(ctx, workspace.Store, evidenceExpected, evidenceDirectories, &report.EvidenceArtifacts, &builder, scanBudget); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return AuthorityHealthReport{}, err
		}
		addHealthError(&builder, err, "artifacts")
	}

	sort.SliceStable(report.Findings, func(i, j int) bool {
		left, right := report.Findings[i], report.Findings[j]
		if left.Severity != right.Severity {
			return healthSeverityRank(left.Severity) > healthSeverityRank(right.Severity)
		}
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		return left.Path < right.Path
	})
	return report, nil
}

func authorityEventHealthFromHistory(history eventHistory, segmentLimit int) AuthorityEventHealth {
	mutationLimit := eventStoreLimit(history.projection)
	remainingBytes := mutationLimit - history.totalBytes
	if remainingBytes < 0 {
		remainingBytes = 0
	}
	remainingSegments := maxEventSegments - history.segmentCount
	if remainingSegments < 0 {
		remainingSegments = 0
	}
	nearBytes := mutationLimit > 0 && history.totalBytes*100 >= mutationLimit*80
	nearSegments := remainingSegments*100 <= maxEventSegments*10
	return AuthorityEventHealth{
		Revision:                      history.projection.Revision,
		EventHead:                     history.projection.EventHead,
		TotalSegments:                 history.segmentCount + 1,
		ContinuationSegments:          history.segmentCount,
		MaximumContinuationSegments:   maxEventSegments,
		RemainingContinuationSegments: remainingSegments,
		CurrentSegmentIndex:           history.currentIndex,
		CurrentSegmentBytes:           history.currentBytes,
		RotationTargetBytes:           int64(segmentLimit),
		TotalBytes:                    history.totalBytes,
		HardLimitBytes:                maxEventStoreBytes,
		CurrentMutationLimitBytes:     mutationLimit,
		RemainingMutationBytes:        remainingBytes,
		TerminalReserveBytes:          terminalEventReserve,
		NearCapacity:                  nearBytes || nearSegments,
	}
}

func verifyReferencedTruthBlobs(ctx context.Context, store *Store, projection Projection, health *AuthorityObjectHealth, builder *authorityHealthBuilder) map[string]struct{} {
	expected := make(map[string]struct{})
	invalidReferences := 0
	for _, acceptance := range projection.TruthHistory {
		for _, file := range acceptance.Files {
			if !isSHA256Digest(file.Digest) {
				health.InvalidReferencedFiles++
				invalidReferences++
				builder.add(AuthorityHealthSeverityIndeterminate, "TRUTH_BLOB_REFERENCE_INVALID", "authority history contains an invalid Project Truth blob digest", file.Path)
				continue
			}
			expected["truth-blobs/"+strings.TrimPrefix(file.Digest, "sha256:")] = struct{}{}
		}
	}
	paths := sortedStringSet(expected)
	health.ReferencedFiles = len(paths) + invalidReferences
	if len(paths) > maxAuthorityExportFiles {
		builder.add(AuthorityHealthSeverityIndeterminate, "AUTHORITY_HEALTH_TOO_MANY_ENTRIES", "referenced Project Truth object count exceeds the supported inventory limit", "truth-blobs")
		return expected
	}
	truthRoot := filepath.Join(store.dir, "truth-blobs")
	rootInfo, rootErr := os.Lstat(truthRoot)
	rootReadable := rootErr == nil && rootInfo.Mode()&os.ModeSymlink == 0 && rootInfo.IsDir() && hasPrivateFilePermissions(rootInfo)
	if !rootReadable {
		health.InvalidReferencedFiles += len(paths)
		if rootErr != nil {
			if os.IsNotExist(rootErr) {
				builder.add(AuthorityHealthSeverityIndeterminate, "TRUTH_BLOB_MISSING", "referenced Project Truth blob store is missing", "truth-blobs")
			} else {
				addHealthError(builder, newError(KindRuntime, "TRUTH_BLOB_STORE_STAT_FAILED", "could not inspect referenced Project Truth blob store", rootErr), "truth-blobs")
			}
		}
		// scanTruthBlobStore records an unsafe existing root. Do not follow an
		// intermediate symlink merely to compute a digest for this report.
		return expected
	}
	for _, relative := range paths {
		select {
		case <-ctx.Done():
			builder.add(AuthorityHealthSeverityIndeterminate, "AUTHORITY_HEALTH_CANCELLED", "Project Truth verification was cancelled", relative)
			return expected
		default:
		}
		digest := "sha256:" + strings.TrimPrefix(relative, "truth-blobs/")
		content, err := store.readTruthBlob(digest)
		if err != nil {
			health.InvalidReferencedFiles++
			addHealthError(builder, err, relative)
			continue
		}
		health.VerifiedFiles++
		health.VerifiedBytes += int64(len(content))
	}
	return expected
}

type authorityEvidenceReference struct {
	digest string
	valid  bool
}

func verifyReferencedEvidence(ctx context.Context, store *Store, projection Projection, health *AuthorityObjectHealth, builder *authorityHealthBuilder) (map[string]struct{}, map[string]struct{}) {
	references := make(map[string]authorityEvidenceReference)
	for _, changeID := range projection.ChangeOrder {
		for _, evidence := range projection.Evidence[changeID] {
			for _, item := range []struct{ path, digest string }{{evidence.StdoutArtifact, evidence.StdoutStoredDigest}, {evidence.StderrArtifact, evidence.StderrStoredDigest}} {
				canonical := isCanonicalEvidenceArtifactPath(item.path)
				validDigest := isSHA256Digest(item.digest)
				current, exists := references[item.path]
				if exists && current.digest != item.digest {
					current.valid = false
					references[item.path] = current
					builder.add(AuthorityHealthSeverityIndeterminate, "EVIDENCE_REFERENCE_COLLISION", "authority history references one Evidence path with different digests", item.path)
					continue
				}
				references[item.path] = authorityEvidenceReference{digest: item.digest, valid: canonical && validDigest}
				if !canonical || !validDigest {
					builder.add(AuthorityHealthSeverityIndeterminate, "EVIDENCE_REFERENCE_INVALID", "authority history contains a non-canonical Evidence artifact reference", item.path)
				}
			}
		}
	}
	paths := make([]string, 0, len(references))
	for path := range references {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	health.ReferencedFiles = len(paths)
	expected := make(map[string]struct{})
	expectedDirectories := map[string]struct{}{"artifacts": {}}
	if len(paths) > maxAuthorityExportFiles {
		builder.add(AuthorityHealthSeverityIndeterminate, "AUTHORITY_HEALTH_TOO_MANY_ENTRIES", "referenced Evidence object count exceeds the supported inventory limit", "artifacts")
		return expected, expectedDirectories
	}
	var referencedBytes int64
	for _, relative := range paths {
		select {
		case <-ctx.Done():
			builder.add(AuthorityHealthSeverityIndeterminate, "AUTHORITY_HEALTH_CANCELLED", "Evidence verification was cancelled", relative)
			return expected, expectedDirectories
		default:
		}
		reference := references[relative]
		if !reference.valid {
			health.InvalidReferencedFiles++
			continue
		}
		expected[relative] = struct{}{}
		parts := strings.Split(relative, "/")
		expectedDirectories[strings.Join(parts[:2], "/")] = struct{}{}
		expectedDirectories[strings.Join(parts[:3], "/")] = struct{}{}
		ancestorsSafe, ancestorErr := privateDirectoryChain(store.dir, parts[:3])
		if ancestorErr != nil {
			health.InvalidReferencedFiles++
			if os.IsNotExist(ancestorErr) {
				builder.add(AuthorityHealthSeverityIndeterminate, "ARTIFACT_MISSING", "referenced Evidence artifact or one of its authority directories is missing", relative)
			} else {
				addHealthError(builder, newError(KindRuntime, "ARTIFACT_ANCESTOR_STAT_FAILED", "could not inspect referenced Evidence artifact directories", ancestorErr), relative)
			}
			continue
		}
		if !ancestorsSafe {
			health.InvalidReferencedFiles++
			builder.add(AuthorityHealthSeverityIndeterminate, "ARTIFACT_UNSAFE", "referenced Evidence artifact has an unsafe authority directory", relative)
			continue
		}
		path := filepath.Join(store.dir, filepath.FromSlash(relative))
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !hasPrivateFilePermissions(info) || info.Size() > maxEvidenceArtifactBytes {
			health.InvalidReferencedFiles++
			if err != nil {
				addHealthError(builder, newError(KindIntegrity, "ARTIFACT_MISSING", "referenced Evidence artifact is missing", err), relative)
			} else {
				builder.add(AuthorityHealthSeverityIndeterminate, "ARTIFACT_UNSAFE", "referenced Evidence artifact is not a bounded private regular file", relative)
			}
			continue
		}
		referencedBytes += info.Size()
		if referencedBytes > maxAuthorityExportBytes {
			health.InvalidReferencedFiles++
			builder.add(AuthorityHealthSeverityIndeterminate, "AUTHORITY_HEALTH_TOO_LARGE", "referenced Evidence artifacts exceed the supported aggregate inventory size", "artifacts")
			break
		}
		if err := store.VerifyArtifact(relative, reference.digest); err != nil {
			health.InvalidReferencedFiles++
			addHealthError(builder, err, relative)
			continue
		}
		health.VerifiedFiles++
		health.VerifiedBytes += info.Size()
	}
	return expected, expectedDirectories
}

func scanAuthorityEventLayout(ctx context.Context, store *Store, health *AuthorityEventHealth, builder *authorityHealthBuilder, budget *authorityHealthBudget) error {
	entries, err := os.ReadDir(store.dir)
	if err != nil {
		return newError(KindRuntime, "AUTHORITY_HEALTH_READ_FAILED", "could not inventory authority state root", err)
	}
	known := map[string]struct{}{
		".gate-run.lock": {}, ".lock": {}, "artifacts": {}, "event-segments": {}, "events.json": {}, "truth-blobs": {},
	}
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		path := filepath.Join(store.dir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return newError(KindRuntime, "AUTHORITY_HEALTH_STAT_FAILED", "could not inspect authority state entry", err)
		}
		if err := budget.consume(info); err != nil {
			return err
		}
		if strings.HasPrefix(entry.Name(), ".write-") {
			if isSafePrivateRegular(info, maxLegacyEventFileBytes) {
				health.TemporaryFiles++
				health.TemporaryBytes += info.Size()
				builder.add(AuthorityHealthSeverityAttention, "EVENT_TEMPORARY_FILE", "an abandoned atomic event-write file remains and is not authority", entry.Name())
			} else {
				health.UnsafeEntries++
				builder.add(AuthorityHealthSeverityIndeterminate, "EVENT_TEMPORARY_FILE_UNSAFE", "an event-write remnant is not a bounded private regular file", entry.Name())
			}
			continue
		}
		if _, ok := known[entry.Name()]; ok {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.Mode().IsRegular() && !info.IsDir()) || !hasPrivateFilePermissions(info) || info.IsDir() {
			health.UnsafeEntries++
			builder.add(AuthorityHealthSeverityIndeterminate, "AUTHORITY_STATE_ENTRY_UNSAFE", "authority state contains an unsafe or unbounded unrecognized entry", entry.Name())
		} else {
			health.UnrecognizedEntries++
			builder.add(AuthorityHealthSeverityAttention, "AUTHORITY_STATE_ENTRY_UNRECOGNIZED", "authority state contains a private regular file with no recognized role", entry.Name())
		}
	}
	segmentDir := filepath.Join(store.dir, "event-segments")
	entries, err = os.ReadDir(segmentDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return newError(KindRuntime, "AUTHORITY_HEALTH_READ_FAILED", "could not inventory event segment directory", err)
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".write-") {
			continue
		}
		path := filepath.Join(segmentDir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return newError(KindRuntime, "AUTHORITY_HEALTH_STAT_FAILED", "could not inspect event segment temporary file", err)
		}
		if err := budget.consume(info); err != nil {
			return err
		}
		if isSafePrivateRegular(info, int64(store.eventSegmentLimit())) {
			health.TemporaryFiles++
			health.TemporaryBytes += info.Size()
			builder.add(AuthorityHealthSeverityAttention, "EVENT_SEGMENT_TEMPORARY_FILE", "an abandoned atomic continuation-segment file remains and is not authority", "event-segments/"+entry.Name())
		} else {
			health.UnsafeEntries++
			builder.add(AuthorityHealthSeverityIndeterminate, "EVENT_SEGMENT_TEMP_UNSAFE", "a continuation-segment remnant is not a bounded private regular file", "event-segments/"+entry.Name())
		}
	}
	return nil
}

func scanTruthBlobStore(ctx context.Context, store *Store, expected map[string]struct{}, health *AuthorityObjectHealth, builder *authorityHealthBuilder, budget *authorityHealthBudget) error {
	root := filepath.Join(store.dir, "truth-blobs")
	info, err := os.Lstat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return newError(KindRuntime, "AUTHORITY_HEALTH_STAT_FAILED", "could not inspect Project Truth blob store", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !hasPrivateFilePermissions(info) {
		health.UnsafeEntries++
		builder.add(AuthorityHealthSeverityIndeterminate, "TRUTH_BLOB_STORE_UNSAFE", "Project Truth blob store is not a private real directory", "truth-blobs")
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return newError(KindRuntime, "AUTHORITY_HEALTH_READ_FAILED", "could not inventory Project Truth blob store", err)
	}
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		path := filepath.Join(root, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return newError(KindRuntime, "AUTHORITY_HEALTH_STAT_FAILED", "could not inspect Project Truth blob entry", err)
		}
		if err := budget.consume(info); err != nil {
			return err
		}
		relative := "truth-blobs/" + entry.Name()
		if strings.HasPrefix(entry.Name(), ".write-") {
			if isSafePrivateRegular(info, maxConfigFileBytes) {
				health.TemporaryEntries++
				health.TemporaryBytes += info.Size()
				builder.add(AuthorityHealthSeverityAttention, "TRUTH_BLOB_TEMPORARY_FILE", "an abandoned Project Truth blob write remains and is not authority", relative)
			} else {
				health.UnsafeEntries++
				builder.add(AuthorityHealthSeverityIndeterminate, "TRUTH_BLOB_TEMP_UNSAFE", "a Project Truth blob remnant is not a bounded private regular file", relative)
			}
			continue
		}
		canonical := len(entry.Name()) == 64 && isSHA256Digest("sha256:"+entry.Name())
		if canonical {
			if _, ok := expected[relative]; ok {
				continue
			}
			if isSafePrivateRegular(info, maxConfigFileBytes) {
				health.OrphanFiles++
				health.OrphanBytes += info.Size()
				builder.add(AuthorityHealthSeverityAttention, "TRUTH_BLOB_ORPHAN", "an unreferenced Project Truth blob remains; health does not delete it", relative)
			} else {
				health.UnsafeEntries++
				builder.add(AuthorityHealthSeverityIndeterminate, "TRUTH_BLOB_ORPHAN_UNSAFE", "an unreferenced Project Truth blob is not a bounded private regular file", relative)
			}
			continue
		}
		if isSafePrivateRegular(info, maxConfigFileBytes) {
			health.UnrecognizedEntries++
			builder.add(AuthorityHealthSeverityAttention, "TRUTH_BLOB_ENTRY_UNRECOGNIZED", "Project Truth blob store contains a private regular file with a non-canonical name", relative)
		} else {
			health.UnsafeEntries++
			builder.add(AuthorityHealthSeverityIndeterminate, "TRUTH_BLOB_ENTRY_UNSAFE", "Project Truth blob store contains an unsafe unrecognized entry", relative)
		}
	}
	return nil
}

func scanEvidenceArtifactStore(ctx context.Context, store *Store, expected, expectedDirectories map[string]struct{}, health *AuthorityObjectHealth, builder *authorityHealthBuilder, budget *authorityHealthBudget) error {
	root := filepath.Join(store.dir, "artifacts")
	info, err := os.Lstat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return newError(KindRuntime, "AUTHORITY_HEALTH_STAT_FAILED", "could not inspect Evidence artifact store", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !hasPrivateFilePermissions(info) {
		health.UnsafeEntries++
		builder.add(AuthorityHealthSeverityIndeterminate, "ARTIFACT_STORE_UNSAFE", "Evidence artifact store is not a private real directory", "artifacts")
		return nil
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return newError(KindRuntime, "AUTHORITY_HEALTH_WALK_FAILED", "could not inventory Evidence artifact store", walkErr)
		}
		if path == root {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		info, err := os.Lstat(path)
		if err != nil {
			return newError(KindRuntime, "AUTHORITY_HEALTH_STAT_FAILED", "could not inspect Evidence artifact entry", err)
		}
		if err := budget.consume(info); err != nil {
			return err
		}
		rel, err := filepath.Rel(store.dir, path)
		if err != nil {
			return newError(KindRuntime, "AUTHORITY_HEALTH_PATH_FAILED", "could not resolve Evidence artifact path", err)
		}
		rel = filepath.ToSlash(rel)
		parts := strings.Split(rel, "/")
		inTemp := len(parts) >= 3 && validateIdentifier(parts[1], "change_id") == nil && strings.HasPrefix(parts[2], ".artifact-")
		if inTemp {
			if len(parts) == 3 {
				health.TemporaryEntries++
				builder.add(AuthorityHealthSeverityAttention, "ARTIFACT_TEMPORARY_DIRECTORY", "an abandoned Evidence staging directory remains and is not authority", rel)
			}
			if info.Mode()&os.ModeSymlink != 0 || (info.IsDir() && !hasPrivateFilePermissions(info)) || (!info.IsDir() && !isSafePrivateRegular(info, maxEvidenceArtifactBytes)) {
				health.UnsafeEntries++
				builder.add(AuthorityHealthSeverityIndeterminate, "ARTIFACT_TEMP_UNSAFE", "an Evidence staging remnant contains an unsafe entry", rel)
				if info.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if info.Mode().IsRegular() {
				health.TemporaryBytes += info.Size()
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) || !hasPrivateFilePermissions(info) {
			health.UnsafeEntries++
			builder.add(AuthorityHealthSeverityIndeterminate, "ARTIFACT_ENTRY_UNSAFE", "Evidence artifact store contains an unsafe entry", rel)
			if info.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			validDirectory := (len(parts) == 2 && validateIdentifier(parts[1], "change_id") == nil) || (len(parts) == 3 && validateIdentifier(parts[1], "change_id") == nil && validateIdentifier(parts[2], "evidence_id") == nil)
			if !validDirectory {
				health.UnrecognizedEntries++
				builder.add(AuthorityHealthSeverityAttention, "ARTIFACT_DIRECTORY_UNRECOGNIZED", "Evidence artifact store contains a private directory with no canonical role", rel)
				return nil
			}
			if _, ok := expectedDirectories[rel]; !ok {
				health.OrphanDirectories++
				builder.add(AuthorityHealthSeverityAttention, "ARTIFACT_ORPHAN_DIRECTORY", "an unreferenced Evidence artifact directory remains; health does not delete it", rel)
			}
			return nil
		}
		if info.Size() > maxEvidenceArtifactBytes {
			health.UnsafeEntries++
			builder.add(AuthorityHealthSeverityIndeterminate, "ARTIFACT_ENTRY_TOO_LARGE", "Evidence artifact file exceeds the supported per-file safety limit", rel)
			return nil
		}
		if _, ok := expected[rel]; ok {
			return nil
		}
		if isCanonicalEvidenceArtifactPath(rel) {
			health.OrphanFiles++
			health.OrphanBytes += info.Size()
			builder.add(AuthorityHealthSeverityAttention, "ARTIFACT_ORPHAN", "an unreferenced Evidence artifact remains; health does not delete it", rel)
		} else {
			health.UnrecognizedEntries++
			builder.add(AuthorityHealthSeverityAttention, "ARTIFACT_ENTRY_UNRECOGNIZED", "Evidence artifact store contains a private regular file with no canonical role", rel)
		}
		return nil
	})
}

func isCanonicalEvidenceArtifactPath(relative string) bool {
	normalized, err := normalizePathRoot(relative)
	if err != nil || normalized != relative {
		return false
	}
	parts := strings.Split(relative, "/")
	return len(parts) == 4 && parts[0] == "artifacts" && validateIdentifier(parts[1], "change_id") == nil && validateIdentifier(parts[2], "evidence_id") == nil && (parts[3] == "stdout.log" || parts[3] == "stderr.log")
}

func isSafePrivateRegular(info os.FileInfo, limit int64) bool {
	return info.Mode()&os.ModeSymlink == 0 && info.Mode().IsRegular() && hasPrivateFilePermissions(info) && info.Size() >= 0 && info.Size() <= limit
}

func privateDirectoryChain(root string, components []string) (bool, error) {
	path := root
	for _, component := range components {
		path = filepath.Join(path, component)
		info, err := os.Lstat(path)
		if err != nil {
			return false, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !hasPrivateFilePermissions(info) {
			return false, nil
		}
	}
	return true, nil
}

func addHealthError(builder *authorityHealthBuilder, err error, path string) {
	var typed *ECPError
	if errors.As(err, &typed) {
		builder.add(AuthorityHealthSeverityIndeterminate, typed.Code, typed.Message, path)
		return
	}
	builder.add(AuthorityHealthSeverityIndeterminate, "AUTHORITY_HEALTH_INSPECTION_FAILED", err.Error(), path)
}

func sortedStringSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func healthSeverityRank(severity AuthorityHealthSeverity) int {
	if severity == AuthorityHealthSeverityIndeterminate {
		return 2
	}
	return 1
}
