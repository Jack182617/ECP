package ecp

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSegmentedEventHistoryRotatesAndReloadsAcrossStoreInstances(t *testing.T) {
	store, projection := createSegmentedStoreFixture(t, 3)

	fresh, err := NewStore(store.baseDir, store.projectID, store.workspaceID, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := fresh.Load(context.Background())
	if err != nil {
		t.Fatalf("fresh Store could not load segmented history: %v", err)
	}
	if reloaded.Revision != projection.Revision || reloaded.EventHead != projection.EventHead {
		t.Fatalf("segmented reload changed the authority head: got revision=%d head=%s, want revision=%d head=%s", reloaded.Revision, reloaded.EventHead, projection.Revision, projection.EventHead)
	}
	if reloaded.AcceptedConfig == nil || projection.AcceptedConfig == nil || reloaded.AcceptedConfig.Reason != projection.AcceptedConfig.Reason {
		t.Fatalf("segmented reload changed the accepted projection: got=%+v want=%+v", reloaded.AcceptedConfig, projection.AcceptedConfig)
	}
	if _, err := os.Lstat(filepath.Join(store.Directory(), "events.json")); err != nil {
		t.Fatalf("rotation removed the compatible root event segment: %v", err)
	}
}

func TestSegmentedEventHistoryDetectsCrossSegmentTampering(t *testing.T) {
	store, _ := createSegmentedStoreFixture(t, 1)
	segmentPath := filepath.Join(store.Directory(), "event-segments", eventSegmentName(1))
	data, err := os.ReadFile(segmentPath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := bytes.Replace(data, []byte(`"origin": "test"`), []byte(`"origin": "tesx"`), 1)
	if bytes.Equal(data, tampered) {
		t.Fatal("test could not locate a continuation event origin to tamper")
	}
	if err := os.WriteFile(segmentPath, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(context.Background()); err == nil || !isErrorCode(err, "EVENT_HASH_INVALID") {
		t.Fatalf("continuation segment tampering was not rejected: %v", err)
	}
}

func TestSegmentedEventHistoryRejectsGapsAndNonCanonicalLayout(t *testing.T) {
	store, _ := createSegmentedStoreFixture(t, 2)
	segmentDir := filepath.Join(store.Directory(), "event-segments")
	if err := os.Rename(filepath.Join(segmentDir, eventSegmentName(1)), filepath.Join(segmentDir, eventSegmentName(999))); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(context.Background()); err == nil || !isErrorCode(err, "EVENT_SEGMENT_LAYOUT_INVALID") {
		t.Fatalf("event segment gap was not rejected: %v", err)
	}
}

func TestSegmentedEventHistoryHandlesOnlySafeAtomicWriteRemnants(t *testing.T) {
	store, projection := createSegmentedStoreFixture(t, 1)
	segmentDir := filepath.Join(store.Directory(), "event-segments")
	safeTemp := filepath.Join(segmentDir, ".write-abandoned")
	if err := os.WriteFile(safeTemp, []byte("incomplete"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("bounded private atomic-write remnant blocked recovery: %v", err)
	}
	if loaded.Revision != projection.Revision || loaded.EventHead != projection.EventHead {
		t.Fatalf("ignored atomic-write remnant changed authority projection: got=%+v want=%+v", loaded, projection)
	}

	unsafeTemp := filepath.Join(segmentDir, ".write-unsafe")
	if err := os.Symlink(filepath.Join(store.Directory(), "events.json"), unsafeTemp); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(context.Background()); err == nil || !isErrorCode(err, "EVENT_SEGMENT_TEMP_UNSAFE") {
		t.Fatalf("unsafe atomic-write remnant was not rejected: %v", err)
	}
}

func TestEventBatchMustFitOneSegmentWithoutPartialMutation(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	if _, err := service.InitProject(ctx, repo, "atomic-event-batch"); err != nil {
		t.Fatal(err)
	}
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
	before, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	store.eventSegmentBytes = 1
	acceptance := *before.AcceptedConfig
	acceptance.Reason = "this event cannot fit in a one-byte segment"
	revision := before.Revision
	if _, err := store.Append(ctx, &revision, PendingEvent{Type: "config_accepted", Origin: "test", Payload: acceptance}); err == nil || !isErrorCode(err, "EVENT_BATCH_TOO_LARGE") {
		t.Fatalf("oversized atomic batch was not rejected: %v", err)
	}

	fresh, err := NewStore(store.baseDir, store.projectID, store.workspaceID, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	after, err := fresh.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || after.EventHead != before.EventHead {
		t.Fatalf("rejected event batch partially mutated authority state: before=%+v after=%+v", before, after)
	}
	if _, err := os.Lstat(filepath.Join(store.Directory(), "event-segments")); !os.IsNotExist(err) {
		t.Fatalf("rejected event batch created continuation state: %v", err)
	}
}

func TestLegacySingleEventFileRemainsCompatible(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	if _, err := service.InitProject(ctx, repo, "legacy-event-file"); err != nil {
		t.Fatal(err)
	}
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
	if _, err := os.Lstat(filepath.Join(store.Directory(), "event-segments")); !os.IsNotExist(err) {
		t.Fatalf("new registration unexpectedly required continuation segments: %v", err)
	}
	projection, err := store.Load(ctx)
	if err != nil || projection.Registration == nil || projection.AcceptedConfig == nil || projection.AcceptedTruth == nil {
		t.Fatalf("legacy single-file authority history did not load: projection=%+v err=%v", projection, err)
	}
}

func createSegmentedStoreFixture(t *testing.T, minimumSegments int) (*Store, Projection) {
	t.Helper()
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	if _, err := service.InitProject(ctx, repo, "segmented-history"); err != nil {
		t.Fatal(err)
	}
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
		acceptance.Reason = fmt.Sprintf("segmented history fixture append %03d", attempt)
		revision := projection.Revision
		projection, err = store.Append(ctx, &revision, PendingEvent{Type: "config_accepted", Origin: "test", Payload: acceptance})
		if err != nil {
			t.Fatalf("could not append segmented history fixture event %d: %v", attempt, err)
		}
		entries, readErr := os.ReadDir(filepath.Join(store.Directory(), "event-segments"))
		if readErr == nil && len(entries) >= minimumSegments {
			return store, projection
		}
		if readErr != nil && !os.IsNotExist(readErr) {
			t.Fatal(readErr)
		}
	}
	t.Fatalf("could not create %d continuation event segments", minimumSegments)
	return nil, Projection{}
}
