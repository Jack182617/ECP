package ecp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestExplicitRegistrationRecoversOnlyTruncatedBindingWithoutHistory(t *testing.T) {
	for _, test := range []struct {
		name    string
		remnant []byte
	}{
		{name: "zero length", remnant: nil},
		{name: "truncated JSON", remnant: []byte(`{"schema_version": 1`)},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			repo := createTestRepository(t)
			service := newTestService(t)
			project, policy, gates := DefaultConfig("binding-recovery", "prj-binding-recovery", time.Now())
			if err := WriteInitialConfig(repo, project, policy, gates); err != nil {
				t.Fatal(err)
			}
			resolved, err := service.withDefaults()
			if err != nil {
				t.Fatal(err)
			}
			workspaceID, err := resolved.Git.WorkspaceID(ctx, repo)
			if err != nil {
				t.Fatal(err)
			}
			bindingPath, err := workspaceBindingPath(resolved.StateDir, workspaceID)
			if err != nil {
				t.Fatal(err)
			}
			if err := ensureAuthorityDirectory(resolved.StateDir, filepath.Dir(bindingPath)); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(bindingPath, test.remnant, 0o600); err != nil {
				t.Fatal(err)
			}
			inspection, err := service.InspectProject(ctx, repo)
			if err != nil || inspection.Registered {
				t.Fatalf("recoverable binding blocked exact inspection: %+v err=%v", inspection, err)
			}
			registered, err := service.RegisterProject(
				ctx, repo, inspection.AuthorityID, inspection.WorkspaceID,
				inspection.CandidateConfigDigest, inspection.CandidateTruthDigest,
				"owner", "recover truncated pre-v0.3 binding after exact review",
			)
			if err != nil || !registered.ConfigAccepted {
				t.Fatalf("explicit registration did not recover truncated binding: %+v err=%v", registered, err)
			}
			binding, err := loadWorkspaceBinding(resolved.StateDir, workspaceID)
			if err != nil || binding.ProjectID != project.ProjectID {
				t.Fatalf("recovered binding is invalid: %+v err=%v", binding, err)
			}
		})
	}
}

func TestBindingRecoveryRefusesValidIncompleteJSONAndExistingHistory(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name          string
		remnant       []byte
		createHistory bool
		code          string
	}{
		{name: "valid incomplete JSON", remnant: []byte("{}\n"), code: "WORKSPACE_BINDING_IDENTITY_MISMATCH"},
		{name: "truncated with history", remnant: []byte(`{"schema_version": 1`), createHistory: true, code: "WORKSPACE_BINDING_RECOVERY_REFUSED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := createTestRepository(t)
			service := newTestService(t)
			project, policy, gates := DefaultConfig("binding-refusal", "prj-binding-refusal", time.Now())
			if err := WriteInitialConfig(repo, project, policy, gates); err != nil {
				t.Fatal(err)
			}
			resolved, err := service.withDefaults()
			if err != nil {
				t.Fatal(err)
			}
			workspaceID, err := resolved.Git.WorkspaceID(ctx, repo)
			if err != nil {
				t.Fatal(err)
			}
			bindingPath, _ := workspaceBindingPath(resolved.StateDir, workspaceID)
			if err := ensureAuthorityDirectory(resolved.StateDir, filepath.Dir(bindingPath)); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(bindingPath, test.remnant, 0o600); err != nil {
				t.Fatal(err)
			}
			if test.createHistory {
				history := filepath.Join(resolved.StateDir, "projects", project.ProjectID, "workspaces", workspaceID)
				if err := ensureAuthorityDirectory(resolved.StateDir, history); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := service.InspectProject(ctx, repo); err == nil || !isErrorCode(err, test.code) {
				t.Fatalf("unsafe binding recovery was not refused with %s: %v", test.code, err)
			}
		})
	}
}

func TestWorkspaceBindingStagingCrashNeverPublishesPartialFinalFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("authority bindings require POSIX Unix semantics")
	}
	directory := t.TempDir()
	marker := filepath.Join(directory, "staged")
	workspaceID := "ws-000000000000000000000001"
	command := exec.Command(os.Args[0], "-test.run=^TestWorkspaceBindingStageCrashHelper$")
	command.Env = append(os.Environ(),
		"ECP_TEST_BINDING_STAGE_DIR="+directory,
		"ECP_TEST_BINDING_STAGE_MARKER="+marker,
	)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waitForTestFile(t, marker, 5*time.Second)
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	target := filepath.Join(directory, workspaceID+".json")
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("killed staging writer exposed a final binding: %v", err)
	}
	// The helper test below validates the actual stage/install split. This parent
	// only asserts the final name is absent after SIGKILL; normal registration
	// recovery is covered by TestExplicitRegistrationRecoversOnlyTruncatedBindingWithoutHistory.
}

func TestWorkspaceBindingStageCrashHelper(t *testing.T) {
	directory := os.Getenv("ECP_TEST_BINDING_STAGE_DIR")
	if directory == "" {
		t.Skip("helper subprocess only")
	}
	staged, err := stageWorkspaceBinding(directory, []byte(`{"schema_version":1`))
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := json.Marshal(map[string]string{"staged": staged})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("ECP_TEST_BINDING_STAGE_MARKER"), metadata, 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Hour)
}

func TestExactRegistrationRetryCompletesDurableBindingBootstrap(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	project, policy, gates := DefaultConfig("binding-bootstrap-retry", "prj-binding-bootstrap-retry", time.Now())
	if err := WriteInitialConfig(repo, project, policy, gates); err != nil {
		t.Fatal(err)
	}
	resolved, err := service.withDefaults()
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := service.InspectProject(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	binding := WorkspaceBinding{
		SchemaVersion:       SchemaVersion,
		AuthorityID:         inspection.AuthorityID,
		WorkspaceID:         inspection.WorkspaceID,
		ProjectID:           inspection.CandidateProjectID,
		InitialConfigDigest: inspection.CandidateConfigDigest,
		InitialTruthDigest:  inspection.CandidateTruthDigest,
		Actor:               "owner",
		Reason:              "resume the exact durable registration",
		Trust:               "local-registration-acknowledgement",
		CoreIdentity:        resolved.CoreIdentity,
		BoundAt:             resolved.Clock().UTC(),
	}
	if err := createWorkspaceBinding(ctx, resolved.StateDir, binding); err != nil {
		t.Fatal(err)
	}

	incompleteInspection, err := service.InspectProject(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if incompleteInspection.Registered || incompleteInspection.BoundProjectID != project.ProjectID {
		t.Fatalf("durable binding without an event batch was reported as registered: %+v", incompleteInspection)
	}
	status, err := service.ProjectStatus(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if status.Registered || status.ConfigState != ProjectConfigUnregistered || len(status.Diagnostics) == 0 || status.Diagnostics[0].Code != "REGISTRATION_BOOTSTRAP_INCOMPLETE" {
		t.Fatalf("incomplete registration status is not actionable: %+v", status)
	}
	if _, err := service.RegisterProject(
		ctx, repo, inspection.AuthorityID, inspection.WorkspaceID,
		inspection.CandidateConfigDigest, inspection.CandidateTruthDigest,
		"another-owner", binding.Reason,
	); err == nil || !isErrorCode(err, "INCOMPLETE_BOOTSTRAP_ACKNOWLEDGEMENT_DRIFT") {
		t.Fatalf("changed acknowledgement resumed an immutable binding: %v", err)
	}
	registered, err := service.RegisterProject(
		ctx, repo, inspection.AuthorityID, inspection.WorkspaceID,
		inspection.CandidateConfigDigest, inspection.CandidateTruthDigest,
		binding.Actor, binding.Reason,
	)
	if err != nil || !registered.ConfigAccepted || !registered.TruthAccepted {
		t.Fatalf("exact retry did not complete registration: %+v err=%v", registered, err)
	}
	// A lost response after the event batch commits is safe to retry with the
	// same acknowledgement and must not duplicate authority events.
	if _, err := service.RegisterProject(
		ctx, repo, inspection.AuthorityID, inspection.WorkspaceID,
		inspection.CandidateConfigDigest, inspection.CandidateTruthDigest,
		binding.Actor, binding.Reason,
	); err != nil {
		t.Fatalf("completed exact registration retry was not idempotent: %v", err)
	}
	store, err := NewStore(resolved.StateDir, project.ProjectID, inspection.WorkspaceID, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Revision != 3 || projection.Registration == nil {
		t.Fatalf("exact retry duplicated or lost bootstrap events: %+v", projection)
	}
}

func TestIncompleteRegistrationRetryRejectsConfigAndCoreDrift(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name   string
		mutate func(*WorkspaceBinding)
		code   string
	}{
		{
			name: "config digest drift",
			mutate: func(binding *WorkspaceBinding) {
				binding.InitialConfigDigest = digestBytes([]byte("different config"))
			},
			code: "INCOMPLETE_BOOTSTRAP_CONFIG_DRIFT",
		},
		{
			name: "Core identity drift",
			mutate: func(binding *WorkspaceBinding) {
				binding.CoreIdentity = "different-core-identity"
			},
			code: "INCOMPLETE_BOOTSTRAP_CORE_DRIFT",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := createTestRepository(t)
			service := newTestService(t)
			project, policy, gates := DefaultConfig("binding-drift", "prj-binding-drift", time.Now())
			if err := WriteInitialConfig(repo, project, policy, gates); err != nil {
				t.Fatal(err)
			}
			resolved, err := service.withDefaults()
			if err != nil {
				t.Fatal(err)
			}
			inspection, err := service.InspectProject(ctx, repo)
			if err != nil {
				t.Fatal(err)
			}
			binding := WorkspaceBinding{
				SchemaVersion:       SchemaVersion,
				AuthorityID:         inspection.AuthorityID,
				WorkspaceID:         inspection.WorkspaceID,
				ProjectID:           inspection.CandidateProjectID,
				InitialConfigDigest: inspection.CandidateConfigDigest,
				InitialTruthDigest:  inspection.CandidateTruthDigest,
				Actor:               "owner",
				Reason:              "exact acknowledgement",
				Trust:               "local-registration-acknowledgement",
				CoreIdentity:        resolved.CoreIdentity,
				BoundAt:             resolved.Clock().UTC(),
			}
			test.mutate(&binding)
			if err := createWorkspaceBinding(ctx, resolved.StateDir, binding); err != nil {
				t.Fatal(err)
			}
			if _, err := service.RegisterProject(
				ctx, repo, inspection.AuthorityID, inspection.WorkspaceID,
				inspection.CandidateConfigDigest, inspection.CandidateTruthDigest,
				"owner", "exact acknowledgement",
			); err == nil || !isErrorCode(err, test.code) {
				t.Fatalf("incomplete binding drift was not rejected with %s: %v", test.code, err)
			}
		})
	}
}

func TestRegistrationWithoutAcceptanceBatchFailsClosed(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	project, policy, gates := DefaultConfig("partial-registration", "prj-partial-registration", time.Now())
	if err := WriteInitialConfig(repo, project, policy, gates); err != nil {
		t.Fatal(err)
	}
	resolved, err := service.withDefaults()
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := service.InspectProject(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	binding := WorkspaceBinding{
		SchemaVersion:       SchemaVersion,
		AuthorityID:         inspection.AuthorityID,
		WorkspaceID:         inspection.WorkspaceID,
		ProjectID:           inspection.CandidateProjectID,
		InitialConfigDigest: inspection.CandidateConfigDigest,
		InitialTruthDigest:  inspection.CandidateTruthDigest,
		Actor:               "owner",
		Reason:              "partial authority fixture",
		Trust:               "local-registration-acknowledgement",
		CoreIdentity:        resolved.CoreIdentity,
		BoundAt:             resolved.Clock().UTC(),
	}
	if err := createWorkspaceBinding(ctx, resolved.StateDir, binding); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(resolved.StateDir, binding.ProjectID, binding.WorkspaceID, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	registration := ProjectRegistration{
		SchemaVersion: SchemaVersion,
		ProjectID:     binding.ProjectID,
		AuthorityID:   binding.AuthorityID,
		WorkspaceID:   binding.WorkspaceID,
		RegisteredAt:  resolved.Clock().UTC(),
		CoreVersion:   CoreVersion,
		CoreIdentity:  resolved.CoreIdentity,
	}
	if _, err := store.Append(ctx, nil, PendingEvent{Type: "project_registered", Origin: "test", Payload: registration}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.InspectProject(ctx, repo); err == nil || !isErrorCode(err, "AUTHORITY_REGISTRATION_INCOMPLETE") {
		t.Fatalf("partial registration was not rejected by inspection: %v", err)
	}
	if _, err := service.ProjectStatus(ctx, repo); err == nil || !isErrorCode(err, "AUTHORITY_REGISTRATION_INCOMPLETE") {
		t.Fatalf("partial registration was not rejected by status: %v", err)
	}
}
