package ecp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxLegacyEventFileBytes        = 64 << 20
	defaultEventSegmentBytes       = 8 << 20
	maxEventSegments               = 1024
	maxEventStoreBytes       int64 = 1 << 30
	terminalEventReserve     int64 = 2 << 20
)

type Store struct {
	baseDir     string
	projectID   string
	workspaceID string
	authorityID string
	dir         string
	lockTimeout time.Duration
	clock       func() time.Time
	// eventSegmentBytes is intentionally unexported. Production stores use the
	// fixed default; tests lower it to exercise rotation without huge fixtures.
	eventSegmentBytes int
}

type PendingEvent struct {
	Type    string
	Origin  string
	Payload any
}

type ArtifactInfo struct {
	StdoutPath   string
	StderrPath   string
	StdoutDigest string
	StderrDigest string
}

type eventHistory struct {
	projection    Projection
	totalBytes    int64
	currentPath   string
	currentIndex  int
	currentBytes  int64
	currentEvents []Event
	segmentCount  int
}

func DefaultStateDir() (string, error) {
	if configured, exists := os.LookupEnv("ECP_STATE_DIR"); exists {
		if err := validateExplicitStateDir(configured); err != nil {
			return "", err
		}
		abs, err := filepath.Abs(configured)
		if err != nil {
			return "", newError(KindRuntime, "STATE_PATH_INVALID", "could not resolve ECP_STATE_DIR", err)
		}
		return filepath.Clean(abs), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", newError(KindRuntime, "HOME_UNAVAILABLE", "could not resolve the user home directory", err)
	}
	return filepath.Join(home, ".ecp", "state-v1"), nil
}

func validateExplicitStateDir(path string) error {
	if path == "" || strings.TrimSpace(path) == "" || strings.ContainsRune(path, '\x00') || !utf8.ValidString(path) {
		return newError(KindUsage, "STATE_PATH_INVALID", "authority state path must be a non-empty valid path", nil)
	}
	return nil
}

func NewStore(baseDir, projectID, workspaceID string, timeout time.Duration) (*Store, error) {
	if err := requireAuthorityPlatform(); err != nil {
		return nil, err
	}
	if err := validateIdentifier(projectID, "project_id"); err != nil {
		return nil, err
	}
	if err := validateIdentifier(workspaceID, "workspace_id"); err != nil {
		return nil, err
	}
	canonicalBaseDir, err := canonicalPotentialPath(baseDir)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	dir := filepath.Join(canonicalBaseDir, "projects", projectID, "workspaces", workspaceID)
	return &Store{
		baseDir:     canonicalBaseDir,
		projectID:   projectID,
		workspaceID: workspaceID,
		authorityID: authorityIDForStateDir(canonicalBaseDir),
		dir:         dir,
		lockTimeout: timeout,
		clock:       time.Now,
	}, nil
}

func (s *Store) Directory() string { return s.dir }

func (s *Store) AcquireGateLease(ctx context.Context) (func(), error) {
	return s.acquireFileLock(
		ctx,
		filepath.Join(s.dir, ".gate-run.lock"),
		s.lockTimeout,
		"GATE_SEQUENCE_LOCKED",
		"another ECP Gate sequence or terminal Change transition is active for this Workspace",
	)
}

// acquireExistingGateLease is the diagnostic counterpart to AcquireGateLease.
// It refuses to create or chmod the authority directory before acquiring the
// advisory lock, so a health inspection cannot silently repair the state it is
// supposed to describe.
func (s *Store) acquireExistingGateLease(ctx context.Context) (func(), error) {
	return s.acquireExistingFileLock(
		ctx,
		filepath.Join(s.dir, ".gate-run.lock"),
		s.lockTimeout,
		"GATE_SEQUENCE_LOCKED",
		"another ECP Gate sequence or terminal Change transition is active for this Workspace",
	)
}

func (s *Store) Exists() bool {
	info, err := os.Lstat(filepath.Join(s.dir, "events.json"))
	return err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0
}

func (s *Store) Load(ctx context.Context) (Projection, error) {
	history, err := s.loadEventHistory(ctx, false)
	if err != nil {
		return Projection{}, err
	}
	return history.projection, nil
}

func (s *Store) Append(ctx context.Context, expectedRevision *uint64, pending ...PendingEvent) (Projection, error) {
	if len(pending) == 0 {
		return Projection{}, newError(KindUsage, "NO_EVENTS", "at least one event is required", nil)
	}
	release, err := s.acquireLock(ctx)
	if err != nil {
		return Projection{}, err
	}
	defer release()

	history, err := s.loadEventHistory(ctx, true)
	if err != nil {
		return Projection{}, err
	}
	projection := history.projection
	if expectedRevision != nil && projection.Revision != *expectedRevision {
		return Projection{}, newError(KindConflict, "STALE_REVISION", "workspace state changed before the requested mutation", nil)
	}
	newEvents := make([]Event, 0, len(pending))
	for _, item := range pending {
		payload, err := json.Marshal(item.Payload)
		if err != nil {
			return Projection{}, newError(KindRuntime, "EVENT_ENCODE_FAILED", "could not encode event payload", err)
		}
		eventID, err := randomID("evt", 12)
		if err != nil {
			return Projection{}, err
		}
		event := Event{
			SchemaVersion: SchemaVersion,
			Sequence:      projection.Revision + 1,
			EventID:       eventID,
			Timestamp:     s.clock().UTC(),
			Type:          item.Type,
			Origin:        item.Origin,
			Payload:       payload,
		}
		event.PreviousHash = projection.EventHead
		event.Hash, err = hashEvent(event)
		if err != nil {
			return Projection{}, newError(KindRuntime, "EVENT_HASH_FAILED", "could not hash event", err)
		}
		if err := applyEvent(&projection, event); err != nil {
			return Projection{}, err
		}
		newEvents = append(newEvents, event)
	}
	if err := validateProjectionStoreIdentity(projection, s.projectID, s.workspaceID, s.authorityID); err != nil {
		return Projection{}, err
	}
	encodedNew, err := encodeEventSegment(newEvents)
	if err != nil {
		return Projection{}, newError(KindRuntime, "STATE_ENCODE_FAILED", "could not encode state events", err)
	}
	segmentLimit := s.eventSegmentLimit()
	if len(encodedNew) > segmentLimit {
		return Projection{}, newError(KindIntegrity, "EVENT_BATCH_TOO_LARGE", "atomic event batch exceeds the segment safety limit", nil)
	}

	combined := append(append([]Event(nil), history.currentEvents...), newEvents...)
	encodedCombined, err := encodeEventSegment(combined)
	if err != nil {
		return Projection{}, newError(KindRuntime, "STATE_ENCODE_FAILED", "could not encode state events", err)
	}
	if len(encodedCombined) <= segmentLimit {
		newTotal := history.totalBytes - history.currentBytes + int64(len(encodedCombined))
		if newTotal > eventStoreLimit(projection) {
			return Projection{}, newError(KindIntegrity, "STATE_TOO_LARGE", "event append would exceed the segmented authority history limit", nil)
		}
		if err := atomicWriteFile(history.currentPath, encodedCombined, 0o600); err != nil {
			return Projection{}, err
		}
		return projection, nil
	}

	if history.segmentCount >= maxEventSegments {
		return Projection{}, newError(KindIntegrity, "STATE_TOO_LARGE", "event history reached the maximum segment count", nil)
	}
	newTotal := history.totalBytes + int64(len(encodedNew))
	if newTotal > eventStoreLimit(projection) {
		return Projection{}, newError(KindIntegrity, "STATE_TOO_LARGE", "event append would exceed the segmented authority history limit", nil)
	}
	segmentDir := filepath.Join(s.dir, "event-segments")
	if err := ensurePrivateDirectory(segmentDir); err != nil {
		return Projection{}, err
	}
	nextIndex := history.currentIndex + 1
	target := filepath.Join(segmentDir, eventSegmentName(nextIndex))
	if _, err := os.Lstat(target); err == nil {
		return Projection{}, newError(KindIntegrity, "EVENT_SEGMENT_EXISTS", "next event segment already exists", nil)
	} else if !os.IsNotExist(err) {
		return Projection{}, newError(KindRuntime, "STATE_STAT_FAILED", "could not inspect next event segment", err)
	}
	if err := atomicWriteFile(target, encodedNew, 0o600); err != nil {
		return Projection{}, err
	}
	return projection, nil
}

func eventStoreLimit(projection Projection) int64 {
	// The reserve exists so an enabled Workspace can always record an audited
	// project-level disablement. Completing or cancelling only the active Change
	// is not enough: ECP is still enabled and must retain that final transition.
	if projection.Activation != nil && !projection.Activation.Enabled && projection.ActiveChange() == nil {
		return maxEventStoreBytes
	}
	return maxEventStoreBytes - terminalEventReserve
}

func (s *Store) eventSegmentLimit() int {
	if s.eventSegmentBytes > 0 {
		return s.eventSegmentBytes
	}
	return defaultEventSegmentBytes
}

func encodeEventSegment(events []Event) ([]byte, error) {
	encoded, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func eventSegmentName(index int) string {
	return fmt.Sprintf("%016d.json", index)
}

func (s *Store) WriteArtifacts(changeID, evidenceID string, stdout, stderr []byte) (ArtifactInfo, error) {
	if err := validateIdentifier(changeID, "change_id"); err != nil {
		return ArtifactInfo{}, err
	}
	if err := validateIdentifier(evidenceID, "evidence_id"); err != nil {
		return ArtifactInfo{}, err
	}
	artifactRoot := filepath.Join(s.dir, "artifacts", changeID)
	if err := ensurePrivateDirectory(artifactRoot); err != nil {
		return ArtifactInfo{}, err
	}
	temp, err := os.MkdirTemp(artifactRoot, ".artifact-")
	if err != nil {
		return ArtifactInfo{}, newError(KindRuntime, "ARTIFACT_TEMP_FAILED", "could not create artifact staging directory", err)
	}
	defer os.RemoveAll(temp)
	if err := os.Chmod(temp, 0o700); err != nil {
		return ArtifactInfo{}, newError(KindRuntime, "ARTIFACT_CHMOD_FAILED", "could not secure artifact directory", err)
	}
	stdoutFile := filepath.Join(temp, "stdout.log")
	stderrFile := filepath.Join(temp, "stderr.log")
	if err := os.WriteFile(stdoutFile, stdout, 0o600); err != nil {
		return ArtifactInfo{}, newError(KindRuntime, "ARTIFACT_WRITE_FAILED", "could not write stdout artifact", err)
	}
	if err := os.WriteFile(stderrFile, stderr, 0o600); err != nil {
		return ArtifactInfo{}, newError(KindRuntime, "ARTIFACT_WRITE_FAILED", "could not write stderr artifact", err)
	}
	target := filepath.Join(artifactRoot, evidenceID)
	if _, err := os.Lstat(target); err == nil {
		return ArtifactInfo{}, newError(KindConflict, "ARTIFACT_EXISTS", "artifact ID already exists", nil)
	} else if !os.IsNotExist(err) {
		return ArtifactInfo{}, newError(KindRuntime, "ARTIFACT_STAT_FAILED", "could not inspect artifact target", err)
	}
	if err := os.Rename(temp, target); err != nil {
		return ArtifactInfo{}, newError(KindRuntime, "ARTIFACT_COMMIT_FAILED", "could not atomically install artifacts", err)
	}
	relStdout, _ := filepath.Rel(s.dir, filepath.Join(target, "stdout.log"))
	relStderr, _ := filepath.Rel(s.dir, filepath.Join(target, "stderr.log"))
	return ArtifactInfo{
		StdoutPath:   filepath.ToSlash(relStdout),
		StderrPath:   filepath.ToSlash(relStderr),
		StdoutDigest: digestBytes(stdout),
		StderrDigest: digestBytes(stderr),
	}, nil
}

func (s *Store) VerifyArtifact(relative, expectedDigest string) error {
	if relative == "" {
		return newError(KindIntegrity, "ARTIFACT_PATH_EMPTY", "evidence artifact path is empty", nil)
	}
	normalized, err := normalizePathRoot(relative)
	if err != nil || normalized == "." {
		return newError(KindIntegrity, "ARTIFACT_PATH_INVALID", "evidence artifact path is unsafe", err)
	}
	path := filepath.Join(s.dir, filepath.FromSlash(normalized))
	rel, err := filepath.Rel(s.dir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return newError(KindIntegrity, "ARTIFACT_PATH_ESCAPE", "evidence artifact path escapes the state directory", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return newError(KindIntegrity, "ARTIFACT_MISSING", "evidence artifact is missing", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !hasPrivateFilePermissions(info) {
		return newError(KindIntegrity, "ARTIFACT_UNSAFE", "evidence artifact is not a regular file", nil)
	}
	digest, err := digestFile(path)
	if err != nil {
		return newError(KindIntegrity, "ARTIFACT_HASH_FAILED", "could not hash evidence artifact", err)
	}
	if digest != expectedDigest {
		return newError(KindIntegrity, "ARTIFACT_DIGEST_MISMATCH", "evidence artifact digest does not match", nil)
	}
	return nil
}

// WriteTruthBlobs persists exact accepted truth bytes in a content-addressed,
// repository-external store. Writing happens before the acceptance event; an
// unreferenced blob left by a later CAS failure is harmless, while an event is
// never allowed to reference content that was not durably written first.
func (s *Store) WriteTruthBlobs(files []TruthFileContent) error {
	if len(files) == 0 || len(files) > maxConfigTreeFiles {
		return newError(KindIntegrity, "TRUTH_BLOB_SET_INVALID", "accepted Project Truth content set is empty or too large", nil)
	}
	root := filepath.Join(s.dir, "truth-blobs")
	if err := ensurePrivateDirectory(root); err != nil {
		return err
	}
	var total int64
	previousPath := ""
	for _, file := range files {
		normalized, err := normalizePathRoot(file.Path)
		if err != nil || normalized != file.Path || (file.Path != "truth.json" && !strings.HasPrefix(file.Path, "contracts/")) || !isSHA256Digest(file.Digest) {
			return newError(KindIntegrity, "TRUTH_BLOB_SET_INVALID", fmt.Sprintf("Project Truth blob %q has an invalid identity", file.Path), err)
		}
		if previousPath != "" && file.Path <= previousPath {
			return newError(KindIntegrity, "TRUTH_BLOB_SET_INVALID", "Project Truth blob set must be unique and sorted", nil)
		}
		previousPath = file.Path
		if len(file.Content) > maxConfigFileBytes || !utf8.Valid(file.Content) || bytes.IndexByte(file.Content, 0) >= 0 || digestBytes(file.Content) != file.Digest {
			return newError(KindIntegrity, "TRUTH_BLOB_CONTENT_INVALID", fmt.Sprintf("Project Truth blob %q does not match its accepted digest or text limits", file.Path), nil)
		}
		total += int64(len(file.Content))
		if total > maxConfigTreeBytes {
			return newError(KindIntegrity, "TRUTH_BLOB_SET_INVALID", "accepted Project Truth blob set exceeds the size limit", nil)
		}
		target := filepath.Join(root, strings.TrimPrefix(file.Digest, "sha256:"))
		if _, err := os.Lstat(target); err == nil {
			if _, err := s.readTruthBlob(file.Digest); err != nil {
				return err
			}
			continue
		} else if !os.IsNotExist(err) {
			return newError(KindRuntime, "TRUTH_BLOB_STAT_FAILED", "could not inspect accepted Project Truth blob", err)
		}
		if err := atomicWriteFile(target, file.Content, 0o600); err != nil {
			return err
		}
		if _, err := s.readTruthBlob(file.Digest); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ReadTruthFiles(files []TruthFileDigest) ([]AcceptedTruthFile, error) {
	if len(files) == 0 || len(files) > maxConfigTreeFiles {
		return nil, newError(KindIntegrity, "TRUTH_BLOB_SET_INVALID", "accepted Project Truth manifest is empty or too large", nil)
	}
	root := filepath.Join(s.dir, "truth-blobs")
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return nil, newError(KindIntegrity, "TRUTH_BLOB_STORE_MISSING", "accepted Project Truth blob store is missing", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() || !hasPrivateFilePermissions(rootInfo) {
		return nil, newError(KindIntegrity, "TRUTH_BLOB_STORE_UNSAFE", "accepted Project Truth blob store is not a private real directory", nil)
	}
	result := make([]AcceptedTruthFile, 0, len(files))
	var total int64
	previousPath := ""
	for _, file := range files {
		normalized, err := normalizePathRoot(file.Path)
		if err != nil || normalized != file.Path || (file.Path != "truth.json" && !strings.HasPrefix(file.Path, "contracts/")) || !isSHA256Digest(file.Digest) {
			return nil, newError(KindIntegrity, "TRUTH_BLOB_MANIFEST_INVALID", fmt.Sprintf("accepted Project Truth file %q has an invalid identity", file.Path), err)
		}
		if previousPath != "" && file.Path <= previousPath {
			return nil, newError(KindIntegrity, "TRUTH_BLOB_MANIFEST_INVALID", "accepted Project Truth manifest must be unique and sorted", nil)
		}
		previousPath = file.Path
		content, err := s.readTruthBlob(file.Digest)
		if err != nil {
			return nil, err
		}
		total += int64(len(content))
		if total > maxConfigTreeBytes {
			return nil, newError(KindIntegrity, "TRUTH_BLOB_SET_INVALID", "accepted Project Truth blobs exceed the size limit", nil)
		}
		result = append(result, AcceptedTruthFile{Path: file.Path, Digest: file.Digest, Content: string(content)})
	}
	return result, nil
}

func (s *Store) readTruthBlob(digest string) ([]byte, error) {
	if !isSHA256Digest(digest) {
		return nil, newError(KindIntegrity, "TRUTH_BLOB_DIGEST_INVALID", "accepted Project Truth blob digest is invalid", nil)
	}
	path := filepath.Join(s.dir, "truth-blobs", strings.TrimPrefix(digest, "sha256:"))
	info, err := os.Lstat(path)
	if err != nil {
		return nil, newError(KindIntegrity, "TRUTH_BLOB_MISSING", "accepted Project Truth content blob is missing", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !hasPrivateFilePermissions(info) || info.Size() > maxConfigFileBytes {
		return nil, newError(KindIntegrity, "TRUTH_BLOB_UNSAFE", "accepted Project Truth content blob is not a bounded private regular file", nil)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, newError(KindRuntime, "TRUTH_BLOB_READ_FAILED", "could not read accepted Project Truth content blob", err)
	}
	if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 || digestBytes(content) != digest {
		return nil, newError(KindIntegrity, "TRUTH_BLOB_DIGEST_MISMATCH", "accepted Project Truth content blob is corrupt", nil)
	}
	return content, nil
}

func (s *Store) loadEventHistory(ctx context.Context, create bool) (eventHistory, error) {
	if create {
		if err := ensurePrivateDirectory(s.dir); err != nil {
			return eventHistory{}, err
		}
	}
	history := eventHistory{
		projection:  NewProjection(),
		currentPath: filepath.Join(s.dir, "events.json"),
	}
	legacyInfo, err := os.Lstat(history.currentPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return eventHistory{}, newError(KindRuntime, "STATE_STAT_FAILED", "could not inspect ECP state", err)
		}
		segmentDir := filepath.Join(s.dir, "event-segments")
		if _, segmentErr := os.Lstat(segmentDir); segmentErr == nil {
			return eventHistory{}, newError(KindIntegrity, "EVENT_SEGMENT_LAYOUT_INVALID", "event segments exist without the required legacy root segment", nil)
		} else if !os.IsNotExist(segmentErr) {
			return eventHistory{}, newError(KindRuntime, "STATE_STAT_FAILED", "could not inspect event segment directory", segmentErr)
		}
		if create {
			return history, nil
		}
		return eventHistory{}, newError(KindNotFound, "STATE_NOT_REGISTERED", "workspace is not registered in the ECP state store", err)
	}
	legacyEvents, legacyBytes, err := readEventSegment(history.currentPath, legacyInfo, maxLegacyEventFileBytes, "events.json")
	if err != nil {
		return eventHistory{}, err
	}
	history.currentEvents = legacyEvents
	history.currentBytes = legacyBytes
	history.totalBytes = legacyBytes
	if err := applyEventSegment(ctx, &history.projection, legacyEvents); err != nil {
		return eventHistory{}, err
	}

	segmentDir := filepath.Join(s.dir, "event-segments")
	segmentInfo, err := os.Lstat(segmentDir)
	if err != nil {
		if os.IsNotExist(err) {
			if err := validateProjectionStoreIdentity(history.projection, s.projectID, s.workspaceID, s.authorityID); err != nil {
				return eventHistory{}, err
			}
			return history, nil
		}
		return eventHistory{}, newError(KindRuntime, "STATE_STAT_FAILED", "could not inspect event segment directory", err)
	}
	if segmentInfo.Mode()&os.ModeSymlink != 0 || !segmentInfo.IsDir() || !hasPrivateFilePermissions(segmentInfo) {
		return eventHistory{}, newError(KindIntegrity, "EVENT_SEGMENT_DIR_UNSAFE", "event-segments must be a private real directory", nil)
	}
	entries, err := os.ReadDir(segmentDir)
	if err != nil {
		return eventHistory{}, newError(KindRuntime, "STATE_READ_FAILED", "could not read event segment directory", err)
	}
	expectedIndex := 1
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return eventHistory{}, ctx.Err()
		default:
		}
		if strings.HasPrefix(entry.Name(), ".write-") {
			path := filepath.Join(segmentDir, entry.Name())
			info, err := os.Lstat(path)
			if err != nil {
				return eventHistory{}, newError(KindRuntime, "STATE_STAT_FAILED", "could not inspect event segment temporary file", err)
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !hasPrivateFilePermissions(info) || info.Size() > int64(s.eventSegmentLimit()) {
				return eventHistory{}, newError(KindIntegrity, "EVENT_SEGMENT_TEMP_UNSAFE", "event segment temporary files must be bounded private regular files", nil)
			}
			continue
		}
		if expectedIndex > maxEventSegments || entry.Name() != eventSegmentName(expectedIndex) {
			return eventHistory{}, newError(KindIntegrity, "EVENT_SEGMENT_LAYOUT_INVALID", "event segment names must be contiguous and canonical", nil)
		}
		path := filepath.Join(segmentDir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return eventHistory{}, newError(KindRuntime, "STATE_STAT_FAILED", "could not inspect event segment", err)
		}
		events, size, err := readEventSegment(path, info, int64(s.eventSegmentLimit()), entry.Name())
		if err != nil {
			return eventHistory{}, err
		}
		if len(events) == 0 {
			return eventHistory{}, newError(KindIntegrity, "EVENT_SEGMENT_EMPTY", "continuation event segments cannot be empty", nil)
		}
		history.totalBytes += size
		if history.totalBytes > maxEventStoreBytes {
			return eventHistory{}, newError(KindIntegrity, "STATE_TOO_LARGE", "segmented authority history exceeds the safety limit", nil)
		}
		if err := applyEventSegment(ctx, &history.projection, events); err != nil {
			return eventHistory{}, err
		}
		history.currentPath = path
		history.currentIndex = expectedIndex
		history.currentBytes = size
		history.currentEvents = events
		history.segmentCount = expectedIndex
		expectedIndex++
	}
	if err := validateProjectionStoreIdentity(history.projection, s.projectID, s.workspaceID, s.authorityID); err != nil {
		return eventHistory{}, err
	}
	return history, nil
}

func readEventSegment(path string, info os.FileInfo, limit int64, label string) ([]Event, int64, error) {
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !hasPrivateFilePermissions(info) {
		return nil, 0, newError(KindIntegrity, "UNSAFE_STATE_FILE", label+" must be a regular private file", nil)
	}
	if info.Size() > limit {
		return nil, 0, newError(KindIntegrity, "STATE_TOO_LARGE", label+" exceeds its event segment safety limit", nil)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, newError(KindRuntime, "STATE_READ_FAILED", "could not read ECP event segment", err)
	}
	if int64(len(data)) > limit {
		return nil, 0, newError(KindIntegrity, "STATE_TOO_LARGE", label+" exceeds its event segment safety limit", nil)
	}
	var events []Event
	if err := decodeStrictJSON(data, &events); err != nil {
		return nil, 0, newError(KindIntegrity, "STATE_JSON_INVALID", label+" is invalid", err)
	}
	return events, int64(len(data)), nil
}

func applyEventSegment(ctx context.Context, projection *Projection, events []Event) error {
	for _, event := range events {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if event.SchemaVersion != SchemaVersion || event.Sequence != projection.Revision+1 {
			return newError(KindIntegrity, "EVENT_SEQUENCE_INVALID", "event sequence or schema is invalid", nil)
		}
		if event.PreviousHash != projection.EventHead {
			return newError(KindIntegrity, "EVENT_CHAIN_INVALID", "event previous_hash does not match", nil)
		}
		expected, err := hashEvent(event)
		if err != nil || expected != event.Hash {
			return newError(KindIntegrity, "EVENT_HASH_INVALID", "event hash does not match its contents", err)
		}
		if err := applyEvent(projection, event); err != nil {
			return err
		}
	}
	return nil
}

func projectEvents(events []Event, projectID, workspaceID, authorityID string) (Projection, error) {
	projection := NewProjection()
	if err := applyEventSegment(context.Background(), &projection, events); err != nil {
		return Projection{}, err
	}
	if err := validateProjectionStoreIdentity(projection, projectID, workspaceID, authorityID); err != nil {
		return Projection{}, err
	}
	return projection, nil
}

func validateProjectionStoreIdentity(projection Projection, projectID, workspaceID, authorityID string) error {
	if projection.Registration == nil {
		return nil
	}
	if projection.Registration.ProjectID != projectID || projection.Registration.WorkspaceID != workspaceID || projection.Registration.AuthorityID != authorityID {
		return newError(KindIntegrity, "STATE_IDENTITY_MISMATCH", "state registration does not match the authority/project/Workspace store path", nil)
	}
	return nil
}

func validateStartedChange(change Change, accepted ConfigAcceptance, acceptedTruth ProjectTruthAcceptance) error {
	if change.SchemaVersion != SchemaVersion || change.State != ChangeActive || change.CreatedAt.IsZero() || change.ConfigDigest != accepted.ConfigDigest ||
		change.TruthDigest != acceptedTruth.TruthDigest ||
		strings.TrimSpace(change.Title) == "" || change.Title != strings.TrimSpace(change.Title) || len(change.Title) > 300 ||
		strings.TrimSpace(change.Goal) == "" || change.Goal != strings.TrimSpace(change.Goal) || len(change.Goal) > 4000 ||
		change.DeclaredRisk.Rank() == 0 || !isSHA256Digest(change.Baseline.Fingerprint) || !isSHA256Digest(change.ContractDigest) {
		return newError(KindIntegrity, "CHANGE_START_INVALID", "Change start event contains an invalid contract, state, epoch, or baseline", nil)
	}
	if change.ContractVersion != 0 && change.ContractVersion != 2 {
		return newError(KindIntegrity, "CHANGE_START_INVALID", "Change start event contains an unsupported contract_version", nil)
	}
	if err := validateIdentifier(change.ChangeID, "change_id"); err != nil {
		return newError(KindIntegrity, "CHANGE_START_INVALID", "Change start event contains an invalid Change ID", err)
	}
	if err := validateIdentifier(change.ActivationID, "activation_id"); err != nil {
		return newError(KindIntegrity, "CHANGE_START_INVALID", "Change start event contains an invalid activation ID", err)
	}
	if err := validateChangeImpact(change.Impact, acceptedTruth.Truth); err != nil {
		return newError(KindIntegrity, "CHANGE_IMPACT_INVALID", "Change impact is invalid or references facts outside the accepted Project Truth", err)
	}
	if err := validateChangeRequirements(change.Requirements, change.AcceptanceCriteria, change.Impact, accepted.Gates, change.ContractVersion >= 2); err != nil {
		return newError(KindIntegrity, "CHANGE_REQUIREMENTS_INVALID", "Change requirement decisions or coverage are invalid", err)
	}
	if change.ContractVersion >= 2 {
		if missing := undeclaredInferredImpact(change.Impact, inferImpact(change.Scope, acceptedTruth.Truth)); !impactInferenceEmpty(missing) {
			return newError(KindIntegrity, "CHANGE_IMPACT_INCOMPLETE", "Change scope implies undeclared or unmapped Project Truth impact", nil)
		}
	}
	normalizedScope, err := normalizeUniquePathRoots(change.Scope)
	if err != nil || len(normalizedScope) == 0 || !slices.Equal(normalizedScope, change.Scope) {
		return newError(KindIntegrity, "CHANGE_START_INVALID", "Change scope is empty, duplicated, or non-canonical", err)
	}
	normalizedNonGoals, err := normalizeTextList(change.NonGoals, false)
	if err != nil || !slices.Equal(normalizedNonGoals, change.NonGoals) {
		return newError(KindIntegrity, "CHANGE_START_INVALID", "Change non-goals are non-canonical", err)
	}
	normalizedAcceptance, err := normalizeTextList(change.AcceptanceCriteria, true)
	if err != nil || !slices.Equal(normalizedAcceptance, change.AcceptanceCriteria) {
		return newError(KindIntegrity, "CHANGE_START_INVALID", "Change acceptance criteria are empty or non-canonical", err)
	}
	expectedContractDigest, err := changeContractDigest(change)
	if err != nil || expectedContractDigest != change.ContractDigest {
		return newError(KindIntegrity, "CHANGE_CONTRACT_DIGEST_INVALID", "Change contract digest does not match its fields", err)
	}
	preflightRisk := MaxRisk(change.DeclaredRisk, impactTruthRisk(change.Impact, acceptedTruth.Truth, false))
	for _, risk := range reachableRisks(preflightRisk, change.Scope, accepted.Policy) {
		if len(requiredGatesForChange(accepted.Gates, acceptedTruth.Truth, acceptedTruth.Truth, change, risk, false, change.Scope)) == 0 {
			return newError(KindIntegrity, "CHANGE_REQUIRED_GATES_MISSING", "Change scope can reach a risk with no required Gate", nil)
		}
	}
	return nil
}

func validateChangeCompletion(completion ChangeCompletion, change *Change, projection Projection) error {
	verdict := completion.FinalVerdict
	if completion.SchemaVersion != SchemaVersion || completion.ChangeID != change.ChangeID || completion.ActivationID != change.ActivationID || completion.CompletedAt.IsZero() ||
		verdict.SchemaVersion != SchemaVersion || verdict.Status != VerdictPass || verdict.ChangeState != ChangeActive || len(verdict.Reasons) != 0 ||
		verdict.ProjectID != projection.Registration.ProjectID || verdict.AuthorityID != projection.Registration.AuthorityID || verdict.WorkspaceID != projection.Registration.WorkspaceID ||
		verdict.ChangeID != change.ChangeID || verdict.ActivationID != change.ActivationID || verdict.ContractDigest != change.ContractDigest || verdict.ConfigDigest != change.ConfigDigest ||
		projection.AcceptedTruth == nil || verdict.TruthDigest != projection.AcceptedTruth.TruthDigest || verdict.SemanticAssessmentID == "" ||
		verdict.DeclaredRisk != change.DeclaredRisk || verdict.EffectiveRisk.Rank() == 0 || !isSHA256Digest(verdict.Source.Fingerprint) || verdict.EvaluatorVersion == "" || verdict.EvaluatedAt.IsZero() {
		return newError(KindIntegrity, "COMPLETION_BINDING_INVALID", "completion Verdict identity, lifecycle, contract, or epoch binding is invalid", nil)
	}
	semantic := latestSemanticAssessment(projection, change.ChangeID)
	if semantic == nil || semantic.AssessmentID != verdict.SemanticAssessmentID || semantic.CurrentTruthDigest != verdict.TruthDigest || semantic.SourceFingerprint != verdict.Source.Fingerprint {
		return newError(KindIntegrity, "COMPLETION_SEMANTIC_INVALID", "completion PASS is not bound to the latest applicable semantic assessment", nil)
	}
	startingTruth, err := acceptedTruthAtDigest(projection, change.TruthDigest)
	if err != nil {
		return err
	}
	truthChanged := change.TruthDigest != projection.AcceptedTruth.TruthDigest
	normalizedTouched, err := normalizeUniquePathRoots(verdict.TouchedPaths)
	expectedRisk := effectiveRiskForContract(*change, verdict.TouchedPaths, projection.AcceptedConfig.Policy, startingTruth, projection.AcceptedTruth.Truth, truthChanged)
	if err != nil || !slices.Equal(normalizedTouched, verdict.TouchedPaths) || verdict.EffectiveRisk != expectedRisk {
		return newError(KindIntegrity, "COMPLETION_RISK_INVALID", "completion Verdict touched paths or effective risk are inconsistent with the Change and policy", err)
	}
	if change.ContractVersion >= 2 {
		expectedInference := inferImpact(verdict.TouchedPaths, projection.AcceptedTruth.Truth)
		if !slices.Equal(verdict.InferredImpact.ComponentIDs, expectedInference.ComponentIDs) ||
			!slices.Equal(verdict.InferredImpact.CapabilityIDs, expectedInference.CapabilityIDs) ||
			!slices.Equal(verdict.InferredImpact.InvariantIDs, expectedInference.InvariantIDs) ||
			!slices.Equal(verdict.InferredImpact.UnmappedPaths, expectedInference.UnmappedPaths) {
			return newError(KindIntegrity, "COMPLETION_IMPACT_INVALID", "completion Verdict inferred impact is inconsistent with its touched paths and accepted Project Truth", nil)
		}
		if missing := undeclaredInferredImpactForChange(*change, expectedInference, startingTruth, truthChanged); !impactInferenceEmpty(missing) {
			return newError(KindIntegrity, "COMPLETION_IMPACT_INVALID", "completion Verdict contains undeclared or unmapped inferred impact", nil)
		}
	}
	expectedSubject, err := verdictSubjectDigestForChange(*change, verdict)
	if err != nil || expectedSubject != verdict.SubjectDigest || expectedSubject != completion.SubjectDigest {
		return newError(KindIntegrity, "COMPLETION_SUBJECT_INVALID", "completion subject digest does not match the final Verdict fields", err)
	}
	verdictDigest, err := digestJSON(verdict)
	if err != nil || verdictDigest != completion.VerdictDigest {
		return newError(KindIntegrity, "COMPLETION_VERDICT_INVALID", "completion Verdict digest does not match the final Verdict payload", err)
	}
	required := requiredGatesForChange(projection.AcceptedConfig.Gates, startingTruth, projection.AcceptedTruth.Truth, *change, verdict.EffectiveRisk, truthChanged, verdict.TouchedPaths)
	if len(required) == 0 || len(verdict.GateAssessments) != len(required) {
		return newError(KindIntegrity, "COMPLETION_GATES_INVALID", "completion PASS does not contain exactly the required Gate assessments", nil)
	}
	assessments := make(map[string]GateAssessment, len(verdict.GateAssessments))
	for _, assessment := range verdict.GateAssessments {
		if _, duplicate := assessments[assessment.GateID]; duplicate {
			return newError(KindIntegrity, "COMPLETION_GATES_INVALID", "completion contains a duplicated Gate assessment", nil)
		}
		assessments[assessment.GateID] = assessment
	}
	for _, gate := range required {
		assessment, ok := assessments[gate.ID]
		if !ok || !assessment.Required || assessment.State != "PASS" || assessment.EvidenceID == "" {
			return newError(KindIntegrity, "COMPLETION_GATES_INVALID", "completion does not contain PASS Evidence for every required Gate", nil)
		}
		matchedEvidence := false
		for _, evidence := range projection.Evidence[change.ChangeID] {
			if evidence.EvidenceID == assessment.EvidenceID && evidence.GateID == gate.ID && evidence.ActivationID == change.ActivationID &&
				evidence.ContractDigest == change.ContractDigest && evidence.ConfigDigest == change.ConfigDigest && evidence.TruthDigest == verdict.TruthDigest &&
				evidence.PreSnapshot.Fingerprint == verdict.Source.Fingerprint && evidence.PostSnapshot.Fingerprint == verdict.Source.Fingerprint && evidence.Passed() {
				matchedEvidence = true
				break
			}
		}
		if !matchedEvidence {
			return newError(KindIntegrity, "COMPLETION_EVIDENCE_INVALID", "completion Gate assessment does not reference applicable passing Evidence", nil)
		}
	}
	if acknowledgementRequired(projection.AcceptedConfig.Policy, verdict.EffectiveRisk) {
		matchedAcknowledgement := false
		for _, acknowledgement := range projection.Acknowledgements[change.ChangeID] {
			if acknowledgement.ID == verdict.Acknowledgement && acknowledgement.ActivationID == change.ActivationID && acknowledgement.SubjectDigest == verdict.SubjectDigest {
				matchedAcknowledgement = true
				break
			}
		}
		if !matchedAcknowledgement {
			return newError(KindIntegrity, "COMPLETION_ACKNOWLEDGEMENT_INVALID", "completion PASS is missing the required subject acknowledgement", nil)
		}
	} else if verdict.Acknowledgement != "" {
		return newError(KindIntegrity, "COMPLETION_ACKNOWLEDGEMENT_INVALID", "completion contains an acknowledgement that policy does not require", nil)
	}
	return nil
}

func applyEvent(projection *Projection, event Event) error {
	decode := func(target any) error {
		if err := decodeStrictJSON(event.Payload, target); err != nil {
			return newError(KindIntegrity, "EVENT_PAYLOAD_INVALID", fmt.Sprintf("event %s payload is invalid", event.Type), err)
		}
		return nil
	}
	if activeRun := projection.ActiveGateRun(); activeRun != nil && event.Type != "evidence_recorded" && event.Type != "gate_run_finished" {
		return newError(KindIntegrity, "EVENT_DURING_ACTIVE_GATE_RUN", "only matching Evidence or the terminal GateRun event may be appended while a GateRun is in progress", nil)
	}
	switch event.Type {
	case "project_registered":
		var registration ProjectRegistration
		if err := decode(&registration); err != nil {
			return err
		}
		if projection.Registration != nil {
			return newError(KindIntegrity, "DUPLICATE_REGISTRATION", "project_registered occurred more than once", nil)
		}
		if registration.SchemaVersion != SchemaVersion || registration.ProjectID == "" || registration.AuthorityID == "" || registration.WorkspaceID == "" || registration.CoreIdentity == "" {
			return newError(KindIntegrity, "REGISTRATION_INVALID", "project registration is incomplete", nil)
		}
		if err := validateIdentifier(registration.ProjectID, "project_id"); err != nil {
			return err
		}
		if err := validateIdentifier(registration.AuthorityID, "authority_id"); err != nil {
			return err
		}
		if err := validateIdentifier(registration.WorkspaceID, "workspace_id"); err != nil {
			return err
		}
		projection.Registration = &registration
	case "project_enabled", "project_disabled":
		var activation ProjectActivation
		if err := decode(&activation); err != nil {
			return err
		}
		if projection.Registration == nil {
			return newError(KindIntegrity, "ACTIVATION_BEFORE_REGISTER", "project activation changed before project registration", nil)
		}
		expectedEnabled := event.Type == "project_enabled"
		if activation.SchemaVersion != SchemaVersion || activation.Enabled != expectedEnabled || activation.ProjectID != projection.Registration.ProjectID ||
			activation.AuthorityID != projection.Registration.AuthorityID || activation.WorkspaceID != projection.Registration.WorkspaceID ||
			strings.TrimSpace(activation.Actor) == "" || strings.TrimSpace(activation.Reason) == "" || activation.Trust == "" ||
			activation.CoreIdentity == "" || activation.ChangedAt.IsZero() {
			return newError(KindIntegrity, "PROJECT_ACTIVATION_INVALID", "project activation is incomplete or bound to the wrong Workspace", nil)
		}
		if err := validateIdentifier(activation.ActivationID, "activation_id"); err != nil {
			return newError(KindIntegrity, "PROJECT_ACTIVATION_ID_INVALID", "project activation ID is invalid", err)
		}
		if _, exists := projection.ActivationIDs[activation.ActivationID]; exists {
			return newError(KindIntegrity, "DUPLICATE_PROJECT_ACTIVATION_ID", "project activation ID was reused", nil)
		}
		expectedPrevious := ""
		if projection.Activation != nil {
			expectedPrevious = projection.Activation.ActivationID
		}
		if activation.PreviousActivation != expectedPrevious {
			return newError(KindIntegrity, "PROJECT_ACTIVATION_CHAIN_INVALID", "project activation does not extend the previous activation epoch", nil)
		}
		if activation.Enabled && (!isSHA256Digest(activation.ConfigDigest) || !isSHA256Digest(activation.TruthDigest)) {
			return newError(KindIntegrity, "PROJECT_ACTIVATION_CONFIG_INVALID", "project enablement is not bound to an accepted config digest", nil)
		}
		if !activation.Enabled && ((activation.ConfigDigest != "" && !isSHA256Digest(activation.ConfigDigest)) || (activation.TruthDigest != "" && !isSHA256Digest(activation.TruthDigest))) {
			return newError(KindIntegrity, "PROJECT_ACTIVATION_CONFIG_INVALID", "project disablement contains an invalid config digest", nil)
		}
		if projection.Activation != nil && projection.Activation.Enabled == activation.Enabled {
			return newError(KindIntegrity, "DUPLICATE_PROJECT_ACTIVATION", "project activation repeated without a state transition", nil)
		}
		if activation.Enabled {
			if projection.AcceptedConfig == nil || activation.ConfigDigest != projection.AcceptedConfig.ConfigDigest {
				return newError(KindIntegrity, "PROJECT_ACTIVATION_CONFIG_MISMATCH", "project enablement does not match the accepted config epoch", nil)
			}
			if projection.AcceptedTruth == nil || activation.TruthDigest != projection.AcceptedTruth.TruthDigest {
				return newError(KindIntegrity, "PROJECT_ACTIVATION_TRUTH_MISMATCH", "project enablement does not match the accepted Project Truth epoch", nil)
			}
			if len(requiredGates(projection.AcceptedConfig.Gates, projection.AcceptedConfig.Policy.DefaultRisk)) == 0 {
				return newError(KindIntegrity, "PROJECT_ACTIVATION_GATES_MISSING", "project enablement has no required Gate for the accepted default risk", nil)
			}
			if projection.ActiveChange() != nil {
				return newError(KindIntegrity, "ENABLE_WITH_ACTIVE_CHANGE", "project was enabled while a Change was already active", nil)
			}
		} else if projection.Activation == nil || !projection.Activation.Enabled {
			return newError(KindIntegrity, "DISABLE_WITHOUT_ENABLE", "project disablement did not follow an enabled epoch", nil)
		} else if projection.AcceptedConfig == nil || activation.ConfigDigest != projection.AcceptedConfig.ConfigDigest || projection.AcceptedTruth == nil || activation.TruthDigest != projection.AcceptedTruth.TruthDigest {
			return newError(KindIntegrity, "PROJECT_ACTIVATION_CONFIG_MISMATCH", "project disablement does not match the accepted config epoch", nil)
		}
		if !activation.Enabled && projection.ActiveChange() != nil {
			return newError(KindIntegrity, "DISABLE_WITH_ACTIVE_CHANGE", "project was disabled while a Change was still active", nil)
		}
		copyOfActivation := activation
		projection.Activation = &copyOfActivation
		projection.ActivationIDs[activation.ActivationID] = struct{}{}
	case "config_accepted":
		var acceptance ConfigAcceptance
		if err := decode(&acceptance); err != nil {
			return err
		}
		if projection.Registration == nil {
			return newError(KindIntegrity, "ACCEPT_BEFORE_REGISTER", "config was accepted before project registration", nil)
		}
		if projection.ActiveChange() != nil {
			return newError(KindIntegrity, "ACCEPT_DURING_ACTIVE_CHANGE", "config acceptance occurred while a Change was active", nil)
		}
		if acceptance.SchemaVersion != SchemaVersion || acceptance.ConfigDigest == "" || acceptance.CoreIdentity == "" {
			return newError(KindIntegrity, "CONFIG_ACCEPTANCE_INVALID", "config acceptance is incomplete", nil)
		}
		if err := validateConfig(acceptance.Project, acceptance.Policy, acceptance.Gates); err != nil {
			return newError(KindIntegrity, "EFFECTIVE_CONFIG_INVALID", "accepted effective config is invalid", err)
		}
		if acceptance.Project.ProjectID != projection.Registration.ProjectID {
			return newError(KindIntegrity, "EFFECTIVE_PROJECT_MISMATCH", "accepted effective project does not match registration", nil)
		}
		projection.AcceptedConfig = &acceptance
		projection.ConfigHistory = append(projection.ConfigHistory, acceptance)
	case "project_truth_accepted":
		var acceptance ProjectTruthAcceptance
		if err := decode(&acceptance); err != nil {
			return err
		}
		if projection.Registration == nil {
			return newError(KindIntegrity, "TRUTH_ACCEPT_BEFORE_REGISTER", "Project Truth was accepted before project registration", nil)
		}
		if err := validateTruthAcceptance(acceptance, projection.AcceptedConfig); err != nil {
			return err
		}
		active := projection.ActiveChange()
		if projection.AcceptedTruth == nil {
			if active != nil || acceptance.ChangeID != "" || acceptance.PreviousTruthDigest != "" {
				return newError(KindIntegrity, "INITIAL_TRUTH_ACCEPTANCE_INVALID", "initial Project Truth acceptance must be part of bootstrap", nil)
			}
		} else {
			if active == nil || acceptance.ChangeID != active.ChangeID || acceptance.PreviousTruthDigest != projection.AcceptedTruth.TruthDigest {
				return newError(KindIntegrity, "TRUTH_ACCEPTANCE_CHANGE_MISMATCH", "Project Truth evolution is not bound to the active Change and previous accepted truth", nil)
			}
			semantic := latestSemanticAssessment(*projection, active.ChangeID)
			if semantic == nil || semantic.PreviousTruthDigest != acceptance.PreviousTruthDigest || semantic.CurrentTruthDigest != acceptance.TruthDigest || semantic.Behavior != SemanticBehaviorChanged {
				return newError(KindIntegrity, "TRUTH_ACCEPTANCE_SEMANTIC_MISMATCH", "Project Truth evolution lacks a matching semantic assessment", nil)
			}
		}
		copyOfAcceptance := acceptance
		projection.AcceptedTruth = &copyOfAcceptance
		projection.TruthHistory = append(projection.TruthHistory, acceptance)
	case "change_started":
		var change Change
		if err := decode(&change); err != nil {
			return err
		}
		if projection.Registration == nil || projection.AcceptedConfig == nil || projection.AcceptedTruth == nil {
			return newError(KindIntegrity, "CHANGE_BEFORE_BOOTSTRAP", "change started before registration, config acceptance, and Project Truth acceptance", nil)
		}
		if !projection.ECPEnabled() {
			return newError(KindIntegrity, "CHANGE_WHILE_ECP_DISABLED", "change started while ECP was disabled for the Workspace", nil)
		}
		if change.ActivationID != projection.Activation.ActivationID {
			return newError(KindIntegrity, "CHANGE_ACTIVATION_MISMATCH", "Change is not bound to the current enabled project epoch", nil)
		}
		if err := validateStartedChange(change, *projection.AcceptedConfig, *projection.AcceptedTruth); err != nil {
			return err
		}
		if change.SupersedesChangeID != "" {
			previous := latestChange(*projection)
			if previous == nil || previous.ChangeID != change.SupersedesChangeID || previous.State != ChangeCancelled || previous.ActivationID != change.ActivationID ||
				previous.Baseline.Fingerprint != change.Baseline.Fingerprint {
				return newError(KindIntegrity, "CHANGE_LINEAGE_INVALID", "superseding Change does not carry the latest cancelled Change baseline", nil)
			}
			expectedRoot := previous.LineageRootChangeID
			if expectedRoot == "" {
				expectedRoot = previous.ChangeID
			}
			if change.LineageRootChangeID != expectedRoot {
				return newError(KindIntegrity, "CHANGE_LINEAGE_INVALID", "superseding Change has an invalid lineage root", nil)
			}
		} else if change.LineageRootChangeID != "" {
			return newError(KindIntegrity, "CHANGE_LINEAGE_INVALID", "non-superseding Change cannot declare a lineage root", nil)
		}
		if _, exists := projection.Changes[change.ChangeID]; exists {
			return newError(KindIntegrity, "DUPLICATE_CHANGE", "change ID is duplicated", nil)
		}
		if projection.ActiveChange() != nil {
			return newError(KindIntegrity, "MULTIPLE_ACTIVE_CHANGES", "more than one active change exists", nil)
		}
		copyOfChange := change
		projection.Changes[change.ChangeID] = &copyOfChange
		projection.ChangeOrder = append(projection.ChangeOrder, change.ChangeID)
	case "gate_run_started":
		var run GateRun
		if err := decode(&run); err != nil {
			return err
		}
		if !projection.ECPEnabled() {
			return newError(KindIntegrity, "GATE_RUN_WHILE_ECP_DISABLED", "GateRun started while ECP was disabled for the Workspace", nil)
		}
		change := projection.Changes[run.ChangeID]
		if change == nil || change.State != ChangeActive {
			return newError(KindIntegrity, "GATE_RUN_WITHOUT_ACTIVE_CHANGE", "GateRun references a missing or non-active Change", nil)
		}
		if run.SchemaVersion != SchemaVersion || run.State != GateRunInProgress || run.ActivationID != projection.Activation.ActivationID || run.ActivationID != change.ActivationID || !isSHA256Digest(run.PlanDigest) || run.StartedAt.IsZero() || !run.FinishedAt.IsZero() || len(run.EvidenceIDs) != 0 || run.OutcomeCode != "" || run.Reason != "" || len(run.GateIDs) == 0 || len(run.GateIDs) > maxConfigTreeFiles {
			return newError(KindIntegrity, "GATE_RUN_START_INVALID", "GateRun start is incomplete or bound to the wrong epoch", nil)
		}
		if err := validateIdentifier(run.RunID, "run_id"); err != nil {
			return newError(KindIntegrity, "GATE_RUN_ID_INVALID", "GateRun ID is invalid", err)
		}
		if _, exists := projection.GateRuns[run.RunID]; exists {
			return newError(KindIntegrity, "DUPLICATE_GATE_RUN", "GateRun ID is duplicated", nil)
		}
		seenGates := make(map[string]struct{}, len(run.GateIDs))
		for _, gateID := range run.GateIDs {
			if err := validateIdentifier(gateID, "gate_id"); err != nil {
				return newError(KindIntegrity, "GATE_RUN_GATE_INVALID", "GateRun contains an invalid Gate ID", err)
			}
			if _, exists := seenGates[gateID]; exists {
				return newError(KindIntegrity, "GATE_RUN_GATE_INVALID", "GateRun contains duplicate Gate IDs", nil)
			}
			seenGates[gateID] = struct{}{}
		}
		copyOfRun := run
		copyOfRun.GateIDs = append([]string(nil), run.GateIDs...)
		copyOfRun.EvidenceIDs = []string{}
		projection.GateRuns[run.RunID] = &copyOfRun
		projection.GateRunOrder = append(projection.GateRunOrder, run.RunID)
	case "evidence_recorded":
		var evidence Evidence
		if err := decode(&evidence); err != nil {
			return err
		}
		if !projection.ECPEnabled() {
			return newError(KindIntegrity, "EVIDENCE_WHILE_ECP_DISABLED", "Evidence was recorded while ECP was disabled for the Workspace", nil)
		}
		change := projection.Changes[evidence.ChangeID]
		if change == nil || change.State != ChangeActive {
			return newError(KindIntegrity, "EVIDENCE_WITHOUT_ACTIVE_CHANGE", "evidence references a missing or non-active change", nil)
		}
		if evidence.SchemaVersion != SchemaVersion || evidence.ProjectID != projection.Registration.ProjectID || evidence.AuthorityID != projection.Registration.AuthorityID || evidence.WorkspaceID != projection.Registration.WorkspaceID ||
			evidence.ActivationID != projection.Activation.ActivationID || evidence.ActivationID != change.ActivationID ||
			!isSHA256Digest(evidence.PlanDigest) ||
			evidence.ConfigDigest != change.ConfigDigest || evidence.TruthDigest != projection.AcceptedTruth.TruthDigest || evidence.ContractDigest != change.ContractDigest || evidence.RunnerVersion == "" || evidence.Scope != "local" {
			return newError(KindIntegrity, "EVIDENCE_BINDING_INVALID", "evidence identity or epoch binding is invalid", nil)
		}
		activeRun := projection.ActiveGateRun()
		if activeRun != nil {
			if evidence.GateRunID != activeRun.RunID || evidence.ChangeID != activeRun.ChangeID || evidence.ActivationID != activeRun.ActivationID || evidence.PlanDigest != activeRun.PlanDigest || !slices.Contains(activeRun.GateIDs, evidence.GateID) {
				return newError(KindIntegrity, "EVIDENCE_GATE_RUN_MISMATCH", "Evidence is not bound to the active GateRun", nil)
			}
		} else if evidence.GateRunID != "" {
			return newError(KindIntegrity, "EVIDENCE_GATE_RUN_MISMATCH", "Evidence references a GateRun that is not active", nil)
		}
		for _, existing := range projection.Evidence[evidence.ChangeID] {
			if existing.EvidenceID == evidence.EvidenceID {
				return newError(KindIntegrity, "DUPLICATE_EVIDENCE", "evidence ID is duplicated", nil)
			}
			if activeRun != nil && existing.GateRunID == activeRun.RunID && existing.GateID == evidence.GateID {
				return newError(KindIntegrity, "DUPLICATE_GATE_RUN_EVIDENCE", "a GateRun may record at most one Evidence item for each selected Gate", nil)
			}
		}
		projection.Evidence[evidence.ChangeID] = append(projection.Evidence[evidence.ChangeID], evidence)
		if activeRun != nil {
			stored := projection.GateRuns[activeRun.RunID]
			copyOfRun := *stored
			copyOfRun.GateIDs = append([]string(nil), stored.GateIDs...)
			copyOfRun.EvidenceIDs = append(append([]string(nil), stored.EvidenceIDs...), evidence.EvidenceID)
			projection.GateRuns[activeRun.RunID] = &copyOfRun
		}
	case "gate_run_finished":
		var terminal GateRunTerminal
		if err := decode(&terminal); err != nil {
			return err
		}
		activeRun := projection.ActiveGateRun()
		if activeRun == nil {
			return newError(KindIntegrity, "GATE_RUN_TERMINAL_WITHOUT_ACTIVE", "GateRun terminal event has no matching active run", nil)
		}
		if terminal.SchemaVersion != SchemaVersion || terminal.RunID != activeRun.RunID || terminal.ChangeID != activeRun.ChangeID || terminal.ActivationID != activeRun.ActivationID || terminal.FinishedAt.IsZero() || terminal.FinishedAt.Before(activeRun.StartedAt) || !validGateRunOutcomeCode(terminal.OutcomeCode) || !validGateRunReason(terminal.Reason) || len(terminal.EvidenceIDs) > len(activeRun.GateIDs) || !slices.Equal(terminal.EvidenceIDs, activeRun.EvidenceIDs) {
			return newError(KindIntegrity, "GATE_RUN_TERMINAL_INVALID", "GateRun terminal event is incomplete or does not match the active run", nil)
		}
		switch terminal.State {
		case GateRunCompleted, GateRunFailed, GateRunCancelled, GateRunInterrupted:
		default:
			return newError(KindIntegrity, "GATE_RUN_TERMINAL_INVALID", "GateRun terminal state is invalid", nil)
		}
		if terminal.State == GateRunCompleted && len(terminal.EvidenceIDs) != len(activeRun.GateIDs) {
			return newError(KindIntegrity, "GATE_RUN_TERMINAL_INVALID", "a completed GateRun must contain one Evidence item for every selected Gate", nil)
		}
		stored := projection.GateRuns[activeRun.RunID]
		copyOfRun := *stored
		copyOfRun.GateIDs = append([]string(nil), stored.GateIDs...)
		copyOfRun.State = terminal.State
		copyOfRun.FinishedAt = terminal.FinishedAt
		copyOfRun.EvidenceIDs = append([]string(nil), terminal.EvidenceIDs...)
		copyOfRun.OutcomeCode = terminal.OutcomeCode
		copyOfRun.Reason = terminal.Reason
		projection.GateRuns[activeRun.RunID] = &copyOfRun
	case "acknowledgement_recorded":
		var acknowledgement Acknowledgement
		if err := decode(&acknowledgement); err != nil {
			return err
		}
		if !projection.ECPEnabled() {
			return newError(KindIntegrity, "ACK_WHILE_ECP_DISABLED", "acknowledgement was recorded while ECP was disabled for the Workspace", nil)
		}
		change := projection.Changes[acknowledgement.ChangeID]
		if change == nil || change.State != ChangeActive {
			return newError(KindIntegrity, "ACK_WITHOUT_ACTIVE_CHANGE", "acknowledgement references a missing or non-active change", nil)
		}
		if acknowledgement.SchemaVersion != SchemaVersion || acknowledgement.ActivationID != projection.Activation.ActivationID || acknowledgement.ActivationID != change.ActivationID {
			return newError(KindIntegrity, "ACK_ACTIVATION_MISMATCH", "acknowledgement is not bound to the current enabled project epoch", nil)
		}
		projection.Acknowledgements[acknowledgement.ChangeID] = append(projection.Acknowledgements[acknowledgement.ChangeID], acknowledgement)
	case "semantic_assessed":
		var assessment SemanticAssessment
		if err := decode(&assessment); err != nil {
			return err
		}
		if !projection.ECPEnabled() || projection.AcceptedTruth == nil {
			return newError(KindIntegrity, "SEMANTIC_ASSESSMENT_WITHOUT_TRUTH", "semantic assessment was recorded without enabled accepted Project Truth", nil)
		}
		change := projection.Changes[assessment.ChangeID]
		if change == nil || change.State != ChangeActive || assessment.ActivationID != projection.Activation.ActivationID || assessment.ActivationID != change.ActivationID {
			return newError(KindIntegrity, "SEMANTIC_ASSESSMENT_CHANGE_MISMATCH", "semantic assessment is not bound to the active Change", nil)
		}
		if err := validateSemanticAssessment(assessment, projection.AcceptedTruth.TruthDigest); err != nil {
			return err
		}
		if err := validateRequirementAssessments(*change, assessment.RequirementAssessments); err != nil {
			return newError(KindIntegrity, "SEMANTIC_REQUIREMENTS_INVALID", "semantic assessment does not reconcile the Change requirements", err)
		}
		for _, existing := range projection.SemanticAssessments[assessment.ChangeID] {
			if existing.AssessmentID == assessment.AssessmentID {
				return newError(KindIntegrity, "DUPLICATE_SEMANTIC_ASSESSMENT", "semantic assessment ID is duplicated", nil)
			}
		}
		projection.SemanticAssessments[assessment.ChangeID] = append(projection.SemanticAssessments[assessment.ChangeID], assessment)
	case "change_completed":
		var completion ChangeCompletion
		if err := decode(&completion); err != nil {
			return err
		}
		if !projection.ECPEnabled() {
			return newError(KindIntegrity, "COMPLETE_WHILE_ECP_DISABLED", "Change completion was recorded while ECP was disabled for the Workspace", nil)
		}
		change := projection.Changes[completion.ChangeID]
		if change == nil || change.State != ChangeActive {
			return newError(KindIntegrity, "COMPLETE_WITHOUT_ACTIVE_CHANGE", "completion references a missing or non-active change", nil)
		}
		if completion.SchemaVersion != SchemaVersion || completion.ActivationID != projection.Activation.ActivationID || completion.ActivationID != change.ActivationID || completion.FinalVerdict.ActivationID != change.ActivationID {
			return newError(KindIntegrity, "COMPLETION_ACTIVATION_MISMATCH", "completion is not bound to the current enabled project epoch", nil)
		}
		if err := validateChangeCompletion(completion, change, *projection); err != nil {
			return err
		}
		copyOfChange := *change
		copyOfChange.State = ChangeCompleted
		projection.Changes[completion.ChangeID] = &copyOfChange
		projection.Completions[completion.ChangeID] = completion
	case "change_cancelled":
		var cancellation ChangeCancellation
		if err := decode(&cancellation); err != nil {
			return err
		}
		change := projection.Changes[cancellation.ChangeID]
		if change == nil || change.State != ChangeActive {
			return newError(KindIntegrity, "CANCEL_WITHOUT_ACTIVE_CHANGE", "cancellation references a missing or non-active Change", nil)
		}
		if cancellation.SchemaVersion != SchemaVersion || cancellation.ActivationID != projection.Activation.ActivationID || cancellation.ActivationID != change.ActivationID || strings.TrimSpace(cancellation.Actor) == "" || strings.TrimSpace(cancellation.Reason) == "" || cancellation.Trust == "" || cancellation.CancelledAt.IsZero() {
			return newError(KindIntegrity, "CHANGE_CANCELLATION_INVALID", "Change cancellation is incomplete", nil)
		}
		copyOfChange := *change
		copyOfChange.State = ChangeCancelled
		projection.Changes[cancellation.ChangeID] = &copyOfChange
		projection.Cancellations[cancellation.ChangeID] = cancellation
	default:
		return newError(KindIntegrity, "UNKNOWN_EVENT_TYPE", fmt.Sprintf("unsupported event type %q", event.Type), nil)
	}
	projection.Revision = event.Sequence
	projection.EventHead = event.Hash
	return nil
}

func validGateRunOutcomeCode(value string) bool {
	if value == "" || len(value) > 200 {
		return false
	}
	for _, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

func validGateRunReason(value string) bool {
	return value != "" && len(value) <= 2000 && value == strings.TrimSpace(value) && utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}

func hashEvent(event Event) (string, error) {
	unsigned := struct {
		SchemaVersion int             `json:"schema_version"`
		Sequence      uint64          `json:"sequence"`
		EventID       string          `json:"event_id"`
		Timestamp     time.Time       `json:"timestamp"`
		Type          string          `json:"type"`
		Origin        string          `json:"origin"`
		PreviousHash  string          `json:"previous_hash"`
		Payload       json.RawMessage `json:"payload"`
	}{
		SchemaVersion: event.SchemaVersion,
		Sequence:      event.Sequence,
		EventID:       event.EventID,
		Timestamp:     event.Timestamp,
		Type:          event.Type,
		Origin:        event.Origin,
		PreviousHash:  event.PreviousHash,
		Payload:       event.Payload,
	}
	return digestJSON(unsigned)
}

func (s *Store) acquireLock(ctx context.Context) (func(), error) {
	return s.acquireFileLock(
		ctx,
		filepath.Join(s.dir, ".lock"),
		s.lockTimeout,
		"WORKSPACE_LOCKED",
		"another ECP mutation is active for this Workspace",
	)
}

func (s *Store) acquireExistingLock(ctx context.Context) (func(), error) {
	return s.acquireExistingFileLock(
		ctx,
		filepath.Join(s.dir, ".lock"),
		s.lockTimeout,
		"WORKSPACE_LOCKED",
		"another ECP mutation is active for this Workspace",
	)
}

func (s *Store) acquireExistingFileLock(ctx context.Context, path string, timeout time.Duration, code, message string) (func(), error) {
	info, err := os.Lstat(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, newError(KindIntegrity, "AUTHORITY_STATE_DIR_MISSING", "authority state directory is missing", err)
		}
		return nil, newError(KindRuntime, "AUTHORITY_STATE_DIR_STAT_FAILED", "could not inspect authority state directory", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !hasPrivateFilePermissions(info) {
		return nil, newError(KindIntegrity, "AUTHORITY_STATE_DIR_UNSAFE", "authority state directory must already be a private real directory", nil)
	}
	return acquirePlatformFileLock(ctx, path, timeout, code, message)
}

func (s *Store) acquireFileLock(ctx context.Context, path string, timeout time.Duration, code, message string) (func(), error) {
	if err := ensurePrivateDirectory(s.dir); err != nil {
		return nil, err
	}
	return acquirePlatformFileLock(ctx, path, timeout, code, message)
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return newError(KindRuntime, "STATE_DIR_CREATE_FAILED", "could not create private ECP state directory", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return newError(KindRuntime, "STATE_DIR_STAT_FAILED", "could not inspect ECP state directory", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return newError(KindIntegrity, "UNSAFE_STATE_DIR", "ECP state directory is not a real directory", nil)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return newError(KindRuntime, "STATE_DIR_CHMOD_FAILED", "could not secure ECP state directory", err)
	}
	return nil
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := ensurePrivateDirectory(dir); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".write-")
	if err != nil {
		return newError(KindRuntime, "ATOMIC_TEMP_FAILED", "could not create atomic write file", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return newError(KindRuntime, "ATOMIC_CHMOD_FAILED", "could not set atomic file permissions", err)
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return newError(KindRuntime, "ATOMIC_WRITE_FAILED", "could not write atomic file", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return newError(KindRuntime, "ATOMIC_SYNC_FAILED", "could not sync atomic file", err)
	}
	if err := temp.Close(); err != nil {
		return newError(KindRuntime, "ATOMIC_CLOSE_FAILED", "could not close atomic file", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return newError(KindRuntime, "ATOMIC_RENAME_FAILED", "could not commit atomic file", err)
	}
	directory, err := os.Open(dir)
	if err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}

func sortedEvidenceByTime(evidence []Evidence) []Evidence {
	result := append([]Evidence(nil), evidence...)
	sort.SliceStable(result, func(i, j int) bool { return result[i].FinishedAt.Before(result[j].FinishedAt) })
	return result
}
