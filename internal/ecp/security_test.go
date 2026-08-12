package ecp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestExistingConfigRequiresExplicitRegistration(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	project, policy, gates := DefaultConfig("preexisting", "prj-preexisting", time.Now())
	if err := WriteInitialConfig(repo, project, policy, gates); err != nil {
		t.Fatal(err)
	}

	if _, err := service.InitProject(ctx, repo, "must-not-bootstrap"); err == nil || !isErrorCode(err, "PROJECT_REGISTRATION_REQUIRED") {
		t.Fatalf("pre-existing Draft Config was implicitly accepted: %v", err)
	}
	workspaceID, err := service.Git.WorkspaceID(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loadWorkspaceBinding(service.StateDir, workspaceID); err == nil || !isErrorCode(err, "WORKSPACE_NOT_REGISTERED") {
		t.Fatalf("project init unexpectedly created an authority binding: %v", err)
	}

	inspection, err := service.InspectProject(ctx, repo)
	if err != nil || inspection.Registered || inspection.CandidateConfigDigest == "" {
		t.Fatalf("could not inspect the unregistered candidate config: %+v err=%v", inspection, err)
	}
	appendFile(t, filepath.Join(repo, ".ecp", "contracts", "boundaries.md"), "changed after review\n")
	if _, err := service.RegisterProject(ctx, repo, inspection.AuthorityID, inspection.WorkspaceID, inspection.CandidateConfigDigest, inspection.CandidateTruthDigest, "owner", "stale review must not bind"); err == nil || !isErrorCode(err, "TRUTH_DIGEST_MISMATCH") {
		t.Fatalf("registration accepted a config that changed after inspection: %v", err)
	}
	if _, err := loadWorkspaceBinding(service.StateDir, workspaceID); err == nil || !isErrorCode(err, "WORKSPACE_NOT_REGISTERED") {
		t.Fatalf("stale registration created an authority binding: %v", err)
	}
	inspection, err = service.InspectProject(ctx, repo)
	if err != nil || inspection.CandidateConfigDigest == "" {
		t.Fatalf("could not re-inspect the changed candidate config: %+v err=%v", inspection, err)
	}
	registered, err := registerTestProject(t, ctx, service, repo, inspection.CandidateConfigDigest, "owner", "reviewed existing Draft Config")
	if err != nil {
		t.Fatalf("explicit registration failed: %v", err)
	}
	if !registered.ConfigAccepted || registered.ProjectID != project.ProjectID {
		t.Fatalf("unexpected registered context: %+v", registered)
	}
}

func TestAuthorityPlatformPolicyFailsClosedForUnsupportedHost(t *testing.T) {
	if err := authorityPlatformRequirement(true); err != nil {
		t.Fatalf("private-authority platform policy was rejected: %v", err)
	}
	if err := authorityPlatformRequirement(false); err == nil || !isErrorCode(err, "PLATFORM_SECURITY_UNSUPPORTED") {
		t.Fatalf("unsupported platform policy did not fail closed before authority work: %v", err)
	}
}

func TestDraftProjectIDCannotReplaceAuthorityIdentity(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	initial, err := service.InitProject(ctx, repo, "identity-test")
	if err != nil {
		t.Fatal(err)
	}
	project := readProjectConfig(t, repo)
	project.ProjectID = "prj-draft-replacement"
	writeProjectConfig(t, repo, project)
	candidate, err := LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}

	current, err := service.Context(ctx, repo)
	if err != nil {
		t.Fatalf("Draft project_id drift lost the authority store: %v", err)
	}
	if current.ProjectID != initial.ProjectID || current.CandidateProjectID != project.ProjectID || current.ConfigAccepted {
		t.Fatalf("Draft project_id replaced authority identity: %+v", current)
	}
	if _, err := acceptTestPolicy(t, ctx, service, repo, candidate.Digest, "owner", "attempt identity replacement"); err == nil || !isErrorCode(err, "PROJECT_ID_IMMUTABLE") {
		t.Fatalf("Draft project_id was accepted: %v", err)
	}
}

func TestConfigReadRejectsMixedEpoch(t *testing.T) {
	repo := createTestRepository(t)
	service := newTestService(t)
	if _, err := service.InitProject(context.Background(), repo, "stable-read"); err != nil {
		t.Fatal(err)
	}
	_, err := loadConfig(repo, func() {
		appendFile(t, filepath.Join(repo, ".ecp", "contracts", "boundaries.md"), "concurrent epoch\n")
	})
	if err == nil || !isErrorCode(err, "CONFIG_CHANGED_DURING_READ") {
		t.Fatalf("mixed configuration epoch was not rejected: %v", err)
	}
}

func TestAuthorityStateAndArtifactsMustRemainPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not authoritative on Windows")
	}
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
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
	if err := os.Chmod(bindingPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Context(ctx, repo); err == nil || !isErrorCode(err, "WORKSPACE_BINDING_UNSAFE") {
		t.Fatalf("world-readable Workspace binding was accepted: %v", err)
	}
	if err := os.Chmod(bindingPath, 0o600); err != nil {
		t.Fatal(err)
	}

	config, err := LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(resolved.StateDir, config.Project.ProjectID, workspaceID, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	eventsPath := filepath.Join(store.Directory(), "events.json")
	if err := os.Chmod(eventsPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Context(ctx, repo); err == nil || !isErrorCode(err, "UNSAFE_STATE_FILE") {
		t.Fatalf("world-readable authority event log was accepted: %v", err)
	}
	if err := os.Chmod(eventsPath, 0o600); err != nil {
		t.Fatal(err)
	}

	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Private artifacts", Goal: "Reject exposed local Evidence artifacts", Scope: []string{"src"},
		AcceptanceCriteria: []string{"artifact permissions remain private"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "private artifact check\n")
	run, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil)
	if err != nil || len(run.Evidence) != 1 {
		t.Fatalf("Gate did not create one Evidence artifact: %+v err=%v", run, err)
	}
	stdoutPath := filepath.Join(store.Directory(), filepath.FromSlash(run.Evidence[0].StdoutArtifact))
	if err := os.Chmod(stdoutPath, 0o644); err != nil {
		t.Fatal(err)
	}
	verdict, err := service.Verdict(ctx, repo)
	if err != nil || verdict.Status != VerdictIndeterminate || !hasReason(verdict, "EVIDENCE_ARTIFACT_INVALID") {
		t.Fatalf("exposed artifact did not fail closed: %+v err=%v", verdict, err)
	}
}

func TestPolicyAcceptanceRequiresInspectedConfigDigest(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	if _, err := service.InitProject(ctx, repo, "config-precondition"); err != nil {
		t.Fatal(err)
	}
	writeGates(t, repo, GatesConfig{SchemaVersion: SchemaVersion, Gates: []GateConfig{passingGitGate()}})
	inspection, err := service.InspectProject(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	changedGates := GatesConfig{SchemaVersion: SchemaVersion, Gates: []GateConfig{passingGitGate()}}
	changedGates.Gates[0].Description = "changed after policy review"
	writeGates(t, repo, changedGates)
	if _, err := acceptTestPolicy(t, ctx, service, repo, inspection.CandidateConfigDigest, "owner", "stale policy review"); err == nil || !isErrorCode(err, "CONFIG_DIGEST_MISMATCH") {
		t.Fatalf("policy acceptance ignored a changed candidate config: %v", err)
	}
	inspection, err = service.InspectProject(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	acceptance, err := acceptTestPolicy(t, ctx, service, repo, inspection.CandidateConfigDigest, "owner", "reviewed exact current config")
	if err != nil || acceptance.ConfigDigest != inspection.CandidateConfigDigest {
		t.Fatalf("exact candidate digest could not be accepted: %+v err=%v", acceptance, err)
	}
}

func TestStartChangeRequiresExactObservedAuthorityWorkspaceConfigAndSource(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	observed, err := service.Context(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	input := StartChangeInput{
		Title: "Observed baseline", Goal: "Bind the exact observed start subject", Scope: []string{"src"},
		AcceptanceCriteria: []string{"unobserved changes are rejected"}, Risk: RiskModerate,
		ExpectedAuthority: observed.AuthorityID, ExpectedWorkspace: observed.WorkspaceID, ExpectedActivation: observed.ActivationToken, ExpectedConfig: observed.CandidateConfig, ExpectedTruth: observed.CandidateTruth, ExpectedSource: observed.Source.Fingerprint,
		Impact: ChangeImpact{Unknowns: []string{"test impact intentionally not modeled"}},
		Requirements: []ChangeRequirement{{
			ID: "observed-start", Statement: "Reject any Change start whose observed subject is stale.", Status: RequirementDecided,
			Verification: RequirementVerificationReview, Rationale: "The test exercises exact start preconditions.", DecisionSource: "security regression contract",
			Covers: RequirementCoverage{AcceptanceCriteria: []string{"unobserved changes are rejected"}, Unknowns: []string{"test impact intentionally not modeled"}},
		}},
	}

	appendFile(t, filepath.Join(repo, "src", "app.txt"), "changed after context\n")
	if _, err := service.StartChange(ctx, repo, input); err == nil || !isErrorCode(err, "SOURCE_FINGERPRINT_MISMATCH") {
		t.Fatalf("Change start absorbed source written after context: %v", err)
	}
	if current, err := service.Context(ctx, repo); err != nil || current.ActiveChange != nil {
		t.Fatalf("stale source precondition mutated lifecycle state: %+v err=%v", current, err)
	}

	observed, err = service.Context(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	changedGates := GatesConfig{SchemaVersion: SchemaVersion, Gates: []GateConfig{passingGitGate()}}
	changedGates.Gates[0].Description = "changed after context"
	writeGates(t, repo, changedGates)
	candidate, err := LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acceptTestPolicy(t, ctx, service, repo, candidate.Digest, "owner", "accept exact new config epoch"); err != nil {
		t.Fatal(err)
	}
	input.ExpectedAuthority = observed.AuthorityID
	input.ExpectedWorkspace = observed.WorkspaceID
	currentStatus, err := service.ProjectStatus(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedActivation = currentStatus.ActivationToken
	input.ExpectedConfig = observed.CandidateConfig
	input.ExpectedSource = observed.Source.Fingerprint
	if _, err := service.StartChange(ctx, repo, input); err == nil || !isErrorCode(err, "CONFIG_DIGEST_MISMATCH") {
		t.Fatalf("Change start absorbed config accepted after context: %v", err)
	}

	otherRepo := createTestRepository(t)
	bootstrapProjectWithGate(t, ctx, service, otherRepo, passingGitGate())
	if _, err := service.StartChange(ctx, otherRepo, input); err == nil || !isErrorCode(err, "WORKSPACE_ID_MISMATCH") {
		t.Fatalf("Change start accepted preconditions from a different Workspace: %v", err)
	}
	if current, err := service.Context(ctx, otherRepo); err != nil || current.ActiveChange != nil {
		t.Fatalf("cross-Workspace precondition mutated the target lifecycle: %+v err=%v", current, err)
	}

	fresh, err := service.Context(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	secondaryAuthority := newTestService(t)
	secondaryInspection, err := secondaryAuthority.InspectProject(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registerTestProject(t, ctx, secondaryAuthority, repo, secondaryInspection.CandidateConfigDigest, "owner", "register the same Workspace in a second local authority"); err != nil {
		t.Fatal(err)
	}
	input.ExpectedAuthority = fresh.AuthorityID
	input.ExpectedWorkspace = fresh.WorkspaceID
	input.ExpectedConfig = fresh.CandidateConfig
	input.ExpectedSource = fresh.Source.Fingerprint
	if _, err := secondaryAuthority.StartChange(ctx, repo, input); err == nil || !isErrorCode(err, "AUTHORITY_ID_MISMATCH") {
		t.Fatalf("Change start accepted preconditions from a different authority store: %v", err)
	}
	if current, err := secondaryAuthority.Context(ctx, repo); err != nil || current.ActiveChange != nil {
		t.Fatalf("cross-authority precondition mutated lifecycle state: %+v err=%v", current, err)
	}

	input.ExpectedAuthority = fresh.AuthorityID
	input.ExpectedWorkspace = fresh.WorkspaceID
	input.ExpectedConfig = fresh.CandidateConfig
	input.ExpectedSource = fresh.Source.Fingerprint
	if _, err := service.StartChange(ctx, repo, input); err != nil {
		t.Fatalf("exact observed start subject was rejected: %v", err)
	}
}

func TestGateRunRequiresExactObservedPlan(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	executableDir := t.TempDir()
	executable := filepath.Join(executableDir, "observed-gate.sh")
	marker := filepath.Join(executableDir, "ran")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf ran > \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	gate := passingGitGate()
	gate.ID = "observed-plan"
	gate.Command = []string{executable, marker}
	bootstrapProjectWithGate(t, ctx, service, repo, gate)
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Plan binding", Goal: "Execute only the exact inspected Gate plan", Scope: []string{"src"},
		AcceptanceCriteria: []string{"stale plans execute no project code"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := service.PlanGates(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if plan.AuthorityID == "" || plan.WorkspaceID == "" || plan.Source.Fingerprint == "" || plan.PlanDigest == "" || len(plan.Gates) != 1 || plan.Gates[0].ResolvedExecutableDigest == "" || plan.Gates[0].EnvironmentDigest == "" {
		t.Fatalf("Gate plan omitted execution-subject bindings: %+v", plan)
	}

	appendFile(t, filepath.Join(repo, "src", "app.txt"), "changed after plan\n")
	result, err := service.RunGates(ctx, repo, change.ChangeID, plan.PlanDigest, nil)
	if err == nil || !isErrorCode(err, "GATE_PLAN_MISMATCH") || len(result.Evidence) != 0 {
		t.Fatalf("Gate run accepted a plan for a different source: result=%+v err=%v", result, err)
	}
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatalf("stale source plan executed Gate code: %v", err)
	}

	plan, err = service.PlanGates(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf changed > \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err = service.RunGates(ctx, repo, change.ChangeID, plan.PlanDigest, nil)
	if err == nil || !isErrorCode(err, "GATE_PLAN_MISMATCH") || len(result.Evidence) != 0 {
		t.Fatalf("Gate run accepted a plan for a replaced executable: result=%+v err=%v", result, err)
	}
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatalf("stale executable plan executed Gate code: %v", err)
	}
	report, err := service.ListEvidence(ctx, repo, change.ChangeID)
	if err != nil || len(report.Evidence) != 0 {
		t.Fatalf("stale plan recorded Evidence: %+v err=%v", report, err)
	}
}

func TestLocalCoreWorktreeCannotRedirectInitialization(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	external := t.TempDir()
	runGit(t, repo, "config", "core.worktree", external)
	service := newTestService(t)

	if _, err := service.InitProject(ctx, repo, "root-boundary"); err == nil || !isErrorCode(err, "GIT_WORKTREE_MISMATCH") {
		t.Fatalf("unsafe core.worktree override was not rejected: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(external, ".ecp")); !os.IsNotExist(err) {
		t.Fatalf("project init wrote outside the physical Git root: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(repo, ".ecp")); !os.IsNotExist(err) {
		t.Fatalf("project init mutated the physical repo before rejecting Git identity: %v", err)
	}
}

func TestCoreGitIgnoresAmbientPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-specific")
	}
	ctx := context.Background()
	repo := createTestRepository(t)
	fakeDirectory := t.TempDir()
	fakeGit := filepath.Join(fakeDirectory, "git")
	marker := filepath.Join(fakeDirectory, "ambient-git-ran")
	if err := os.WriteFile(fakeGit, []byte("#!/bin/sh\nprintf ran > \"$ECP_FAKE_GIT_MARKER\"\nexit 99\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ECP_FAKE_GIT_MARKER", marker)
	t.Setenv("PATH", fakeDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, policy, _ := DefaultConfig("trusted-git", "prj-trusted-git", time.Now())
	snapshot, err := (Git{}).Snapshot(ctx, repo, policy)
	if err != nil {
		t.Fatalf("fixed system Git could not inspect the repository: %v", err)
	}
	if snapshot.GitExecutable == "" || snapshot.GitExecutableDigest == "" || snapshot.Ref().GitExecutableDigest != snapshot.GitExecutableDigest {
		t.Fatalf("source snapshot omitted trusted Git identity: %+v", snapshot.Ref())
	}
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatalf("Core executed a Git shim from ambient PATH: %v", err)
	}
}

func TestSubmoduleCheckoutChangesSourceFingerprint(t *testing.T) {
	ctx := context.Background()
	submoduleRepo := createCommitSeriesRepository(t)
	commitA := gitOutput(t, submoduleRepo, "rev-list", "--max-parents=0", "HEAD")
	commitC := gitOutput(t, submoduleRepo, "rev-parse", "HEAD")
	commitB := gitOutput(t, submoduleRepo, "rev-parse", "HEAD~1")
	runGit(t, submoduleRepo, "checkout", commitA)

	parent := createTestRepository(t)
	runGit(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", submoduleRepo, "deps/sub")
	runGit(t, parent, "commit", "-am", "add submodule")
	_, budgetPolicy, _ := DefaultConfig("submodule-budget", "prj-submodule-budget", time.Now())
	budgetPolicy.MaxSourceFiles = 4
	if _, err := (Git{}).Snapshot(ctx, parent, budgetPolicy); err == nil || !isErrorCode(err, "SUBMODULE_SNAPSHOT_FAILED") || !strings.Contains(err.Error(), "SOURCE_FILE_LIMIT") {
		t.Fatalf("recursive submodule files did not share the top-level source budget: %v", err)
	}
	_, normalPolicy, _ := DefaultConfig("submodule-availability", "prj-submodule-availability", time.Now())
	submodulePath := filepath.Join(parent, "deps", "sub")
	movedSubmodule := filepath.Join(parent, "deps", "sub-moved")
	if err := os.Rename(submodulePath, movedSubmodule); err != nil {
		t.Fatal(err)
	}
	if _, err := (Git{}).Snapshot(ctx, parent, normalPolicy); err == nil || !isErrorCode(err, "SUBMODULE_UNAVAILABLE") {
		t.Fatalf("uninitialized indexed submodule was fingerprinted as absent: %v", err)
	}
	if err := os.WriteFile(submodulePath, []byte("not a submodule\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (Git{}).Snapshot(ctx, parent, normalPolicy); err == nil || !isErrorCode(err, "SUBMODULE_UNAVAILABLE") {
		t.Fatalf("indexed submodule replaced by a regular file was accepted: %v", err)
	}
	if err := os.Remove(submodulePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(movedSubmodule, submodulePath); err != nil {
		t.Fatal(err)
	}
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, parent, passingGitGate())
	change, err := startTestChange(t, ctx, service, parent, StartChangeInput{
		Title: "Submodule update", Goal: "Bind Gate evidence to the exact submodule checkout", Scope: []string{"deps"},
		AcceptanceCriteria: []string{"source check passes"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, filepath.Join(parent, "deps", "sub"), "checkout", commitB)
	if _, err := runTestGates(t, ctx, service, parent, change.ChangeID, nil); err != nil {
		t.Fatal(err)
	}
	passed, err := service.Verdict(ctx, parent)
	if err != nil || passed.Status != VerdictPass {
		t.Fatalf("submodule B precondition did not PASS: %+v %v", passed, err)
	}
	runGit(t, filepath.Join(parent, "deps", "sub"), "checkout", commitC)
	stale, err := service.Verdict(ctx, parent)
	if err != nil || stale.Status != VerdictBlocked || !hasReason(stale, "GATE_EVIDENCE_STALE") {
		t.Fatalf("Evidence from submodule B applied to C: %+v %v", stale, err)
	}
	if _, err := cancelTestChange(t, ctx, service, parent, change.ChangeID, "owner", "prepare nested denied-path regression"); err != nil {
		t.Fatal(err)
	}
	deniedCandidate, err := LoadConfig(parent)
	if err != nil {
		t.Fatal(err)
	}
	deniedCandidate.Policy.DeniedPathRoots = append(deniedCandidate.Policy.DeniedPathRoots, "deps/sub/security")
	if err := writePrettyJSON(filepath.Join(parent, ".ecp", "policy.json"), deniedCandidate.Policy, 0o644); err != nil {
		t.Fatal(err)
	}
	deniedCandidate, err = LoadConfig(parent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acceptTestPolicy(t, ctx, service, parent, deniedCandidate.Digest, "owner", "accept nested submodule deny rule"); err != nil {
		t.Fatal(err)
	}
	deniedChange, err := startTestChange(t, ctx, service, parent, StartChangeInput{
		Title: "Nested denied submodule path", Goal: "Conservatively block collapsed submodule changes", Scope: []string{"deps"},
		AcceptanceCriteria: []string{"nested denied path blocks"}, Risk: RiskModerate, SupersedesChangeID: change.ChangeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, filepath.Join(parent, "deps", "sub"), "checkout", commitB)
	if _, err := runTestGates(t, ctx, service, parent, deniedChange.ChangeID, nil); err != nil {
		t.Fatal(err)
	}
	deniedVerdict, err := service.Verdict(ctx, parent)
	if err != nil || deniedVerdict.Status != VerdictBlocked || !hasReason(deniedVerdict, "DENIED_PATH_TOUCHED") {
		t.Fatalf("nested denied submodule path did not block: %+v err=%v", deniedVerdict, err)
	}
}

func TestStateDirectoryInsideRepositoryIsRejectedBeforeConfigWrite(t *testing.T) {
	repo := createTestRepository(t)
	service := Service{StateDir: filepath.Join(repo, ".ecp-state")}
	if _, err := service.InitProject(context.Background(), repo, "unsafe-state"); err == nil || !isErrorCode(err, "STATE_INSIDE_REPOSITORY") {
		t.Fatalf("repository-local authority state was accepted: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(repo, ".ecp")); !os.IsNotExist(err) {
		t.Fatalf("Init mutated the repository before rejecting authority state: %v", err)
	}
}

func TestStateDirectoryAlternateFilesystemSpellingIsRejected(t *testing.T) {
	repo := createTestRepository(t)
	alternate := toggleFirstASCIIPathCase(repo)
	repoInfo, err := os.Stat(repo)
	if err != nil {
		t.Fatal(err)
	}
	alternateInfo, err := os.Stat(alternate)
	if err != nil || !os.SameFile(repoInfo, alternateInfo) {
		t.Skip("filesystem does not expose an alternate-case spelling for this path")
	}
	stateDir := filepath.Join(alternate, ".state-hidden")
	service := Service{StateDir: stateDir}
	if _, err := service.InitProject(context.Background(), repo, "unsafe-state-alias"); err == nil || !isErrorCode(err, "STATE_INSIDE_REPOSITORY") {
		t.Fatalf("alternate spelling of repository-local authority state was accepted: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(repo, ".ecp")); !os.IsNotExist(err) {
		t.Fatalf("Init mutated the repository before rejecting authority state alias: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(repo, ".state-hidden")); !os.IsNotExist(err) {
		t.Fatalf("Init wrote authority state through an alternate repository spelling: %v", err)
	}
}

func TestStateDirectoryEnvironmentPreservesSignificantWhitespace(t *testing.T) {
	base := t.TempDir()
	expected := filepath.Join(base, "authority-state ")
	t.Setenv("ECP_STATE_DIR", expected)
	configured, err := DefaultStateDir()
	if err != nil {
		t.Fatal(err)
	}
	if configured != expected {
		t.Fatalf("ECP_STATE_DIR path bytes were changed: got %q want %q", configured, expected)
	}
	repo := createTestRepository(t)
	if _, err := (Service{}).InitProject(context.Background(), repo, "space-state"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(expected); err != nil || !info.IsDir() {
		t.Fatalf("authority state was not created at the exact configured path %q: %v", expected, err)
	}
	if _, err := os.Stat(strings.TrimSuffix(expected, " ")); !os.IsNotExist(err) {
		t.Fatalf("trimmed authority path was unexpectedly selected: %v", err)
	}
}

func TestStateDirectoryEnvironmentRejectsBlankValue(t *testing.T) {
	t.Setenv("ECP_STATE_DIR", "   ")
	if _, err := DefaultStateDir(); err == nil || !isErrorCode(err, "STATE_PATH_INVALID") {
		t.Fatalf("blank ECP_STATE_DIR did not fail closed: %v", err)
	}
	t.Setenv("ECP_STATE_DIR", t.TempDir())
	if _, err := (Service{StateDir: "   "}).withDefaults(); err == nil || !isErrorCode(err, "STATE_PATH_INVALID") {
		t.Fatalf("blank explicit Service.StateDir silently selected the default authority: %v", err)
	}
}

func TestTrailingSpaceScopeRemainsAnExactPath(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	plainDirectory := filepath.Join(repo, "dir")
	spaceDirectory := filepath.Join(repo, "dir ")
	for _, directory := range []string{plainDirectory, spaceDirectory} {
		if err := os.Mkdir(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "value.txt"), []byte("baseline\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, repo, "add", "dir", "dir ")
	runGit(t, repo, "commit", "-m", "add exact path fixtures")
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Exact trailing-space scope", Goal: "Keep distinct repository path bytes distinct", Scope: []string{"dir "},
		AcceptanceCriteria: []string{"only the trailing-space directory is in scope"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(change.Scope) != 1 || change.Scope[0] != "dir " {
		t.Fatalf("Change scope silently changed path bytes: %#v", change.Scope)
	}
	appendFile(t, filepath.Join(spaceDirectory, "value.txt"), "in scope\n")
	if _, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil); err != nil {
		t.Fatal(err)
	}
	assertVerdictStatus(t, service, repo, VerdictPass)
	appendFile(t, filepath.Join(plainDirectory, "value.txt"), "outside scope\n")
	if _, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil); err != nil {
		t.Fatal(err)
	}
	assertVerdictReason(t, service, repo, VerdictBlocked, "OUT_OF_SCOPE")
}

func toggleFirstASCIIPathCase(path string) string {
	bytes := []byte(path)
	for index, value := range bytes {
		switch {
		case value >= 'a' && value <= 'z':
			bytes[index] = value - ('a' - 'A')
			return string(bytes)
		case value >= 'A' && value <= 'Z':
			bytes[index] = value + ('a' - 'A')
			return string(bytes)
		}
	}
	return path
}

func TestGateArgvMetacharactersRemainLiteral(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	marker := filepath.Join(t.TempDir(), "must-not-exist")
	gate := passingGitGate()
	gate.Command = []string{"printf", "%s", "literal;touch " + marker}
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, gate)
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Literal argv", Goal: "Do not invoke an implicit shell", Scope: []string{"src"},
		AcceptanceCriteria: []string{"metacharacters remain one argument"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatalf("shell metacharacters executed a second command: %v", err)
	}
	if len(run.Evidence) != 1 || !run.Evidence[0].Passed() || run.Evidence[0].Command[2] != "literal;touch "+marker {
		t.Fatalf("argv evidence was not literal: %+v", run)
	}
}

func TestRunnerTimeoutKillsDescendantsAndCapsOutput(t *testing.T) {
	repo := createTestRepository(t)
	canonicalRepo, err := (Git{}).DiscoverRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	_, policy, _ := DefaultConfig("runner", "prj-runner", time.Now())
	timeoutGate := passingGitGate()
	timeoutGate.Command = []string{"sh", "-c", "sleep 5 & wait"}
	timeoutGate.TimeoutSeconds = 1
	started := time.Now()
	execution, err := (Runner{}).Run(context.Background(), canonicalRepo, policy, timeoutGate)
	if err != nil {
		t.Fatal(err)
	}
	if !execution.TimedOut || execution.PassedForTest() || time.Since(started) > 3*time.Second {
		t.Fatalf("timeout did not bound the process tree: elapsed=%s execution=%+v", time.Since(started), execution)
	}

	outputGate := passingGitGate()
	outputGate.Command = []string{"sh", "-c", `i=0; while [ "$i" -lt 5000 ]; do printf x; i=$((i+1)); done`}
	outputGate.MaxOutputBytes = 1024
	output, err := (Runner{}).Run(context.Background(), canonicalRepo, policy, outputGate)
	if err != nil {
		t.Fatal(err)
	}
	if !output.ExitCodeAllowed || output.StdoutBytes != 5000 || len(output.Stdout) != 1024 || !output.StdoutTruncated {
		t.Fatalf("bounded output metadata is wrong: %+v stored=%d", output, len(output.Stdout))
	}
}

func (e Execution) PassedForTest() bool {
	return e.ExitCodeAllowed && !e.TimedOut && e.ProcessError == ""
}

func TestEvidenceStalesOnExecutableEnvironmentAndCoreIdentity(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	executable := filepath.Join(t.TempDir(), "gate.sh")
	writeExecutable(t, executable, "#!/bin/sh\nexit 0\n")
	t.Setenv("ECP_TEST_CONTEXT_VALUE", "epoch-one-unique")
	gate := passingGitGate()
	gate.Command = []string{executable}
	gate.InheritEnvironment = []string{"ECP_TEST_CONTEXT_VALUE"}
	service := newTestService(t)
	service.CoreIdentity = "test-core-identity-a"
	bootstrapProjectWithGate(t, ctx, service, repo, gate)
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Execution context", Goal: "Bind exact execution context", Scope: []string{"src"},
		AcceptanceCriteria: []string{"context-bound Gate passes"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil); err != nil {
		t.Fatal(err)
	}
	assertVerdictStatus(t, service, repo, VerdictPass)

	writeExecutable(t, executable, "#!/bin/sh\n# different executable bytes\nexit 0\n")
	assertVerdictReason(t, service, repo, VerdictBlocked, "GATE_EVIDENCE_STALE")
	if _, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil); err != nil {
		t.Fatal(err)
	}
	assertVerdictStatus(t, service, repo, VerdictPass)

	t.Setenv("ECP_TEST_CONTEXT_VALUE", "epoch-two-unique")
	assertVerdictReason(t, service, repo, VerdictBlocked, "GATE_EVIDENCE_STALE")
	t.Setenv("ECP_TEST_CONTEXT_VALUE", "epoch-one-unique")
	assertVerdictStatus(t, service, repo, VerdictPass)

	otherBuild := service
	otherBuild.CoreIdentity = "test-core-identity-b"
	assertVerdictReason(t, otherBuild, repo, VerdictBlocked, "GATE_EVIDENCE_STALE")

	workspaceID, err := service.Git.WorkspaceID(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(service.StateDir, config.Project.ProjectID, workspaceID, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	events, err := os.ReadFile(filepath.Join(store.Directory(), "events.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(events, []byte("epoch-one-unique")) {
		t.Fatal("environment value leaked into authority events")
	}
}

func TestGateFailureModesFailClosed(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	gates := []GateConfig{
		gateWithCommand("exit-seven", []string{"sh", "-c", "printf 'PASS approved'; exit 7"}),
		gateWithCommand("timeout", []string{"sh", "-c", "while :; do :; done"}),
	}
	gates[1].TimeoutSeconds = 1
	bootstrapProjectWithGates(t, ctx, service, repo, gates)
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Failure modes", Goal: "Every process failure fails closed", Scope: []string{"src"},
		AcceptanceCriteria: []string{"all failure modes are non-passing"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := runTestGates(t, ctx, service, repo, change.ChangeID, []string{"exit-seven"})
	if err != nil || len(failed.Evidence) != 1 || failed.Evidence[0].Passed() {
		t.Fatalf("non-allowed exit did not create failed Evidence: %+v %v", failed, err)
	}
	timed, err := runTestGates(t, ctx, service, repo, change.ChangeID, []string{"timeout"})
	if err != nil || len(timed.Evidence) != 1 || !timed.Evidence[0].TimedOut || timed.Evidence[0].Passed() {
		t.Fatalf("timeout did not create failed Evidence: %+v %v", timed, err)
	}
	verdict, err := service.Verdict(ctx, repo)
	if err != nil || verdict.Status == VerdictPass {
		t.Fatalf("failure modes yielded PASS: %+v %v", verdict, err)
	}
}

func TestPathAndSymlinkBoundaries(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	if _, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Traversal", Goal: "Reject path traversal", Scope: []string{"../outside"},
		AcceptanceCriteria: []string{"rejected"}, Risk: RiskModerate,
	}); err == nil || !isErrorCode(err, "PATH_TRAVERSAL") {
		t.Fatalf("path traversal scope was accepted: %v", err)
	}

	external := t.TempDir()
	if err := os.Symlink(external, filepath.Join(repo, "linked-cwd")); err != nil {
		t.Fatal(err)
	}
	_, policy, _ := DefaultConfig("path", "prj-path", time.Now())
	gate := passingGitGate()
	gate.WorkingDirectory = "linked-cwd"
	if _, err := (Runner{}).Run(ctx, repo, policy, gate); err == nil || !isErrorCode(err, "GATE_CWD_ESCAPE") {
		t.Fatalf("symlink cwd escape was accepted: %v", err)
	}

	target := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(target, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(repo, "src", "outside-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	before, err := service.Git.Snapshot(ctx, repo, policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	afterTargetChange, err := service.Git.Snapshot(ctx, repo, policy)
	if err != nil {
		t.Fatal(err)
	}
	if before.Fingerprint != afterTargetChange.Fingerprint {
		t.Fatal("source fingerprint followed a symlink target outside the repository")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target+"-different", link); err != nil {
		t.Fatal(err)
	}
	afterLinkChange, err := service.Git.Snapshot(ctx, repo, policy)
	if err != nil {
		t.Fatal(err)
	}
	if before.Fingerprint == afterLinkChange.Fingerprint {
		t.Fatal("symlink link text change did not change source fingerprint")
	}
}

func TestEventTamperRevisionCASAndGateLease(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	if _, err := service.InitProject(ctx, repo, "store-security"); err != nil {
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
	store, err := NewStore(service.StateDir, config.Project.ProjectID, workspaceID, 150*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	acceptance := *projection.AcceptedConfig
	acceptance.Reason = "first CAS append"
	revision := projection.Revision
	if _, err := store.Append(ctx, &revision, PendingEvent{Type: "config_accepted", Origin: "test", Payload: acceptance}); err != nil {
		t.Fatal(err)
	}
	acceptance.Reason = "stale CAS append"
	if _, err := store.Append(ctx, &revision, PendingEvent{Type: "config_accepted", Origin: "test", Payload: acceptance}); err == nil || !isErrorCode(err, "STALE_REVISION") {
		t.Fatalf("stale revision append succeeded: %v", err)
	}

	release, err := store.AcquireGateLease(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcquireGateLease(ctx); err == nil || !isErrorCode(err, "GATE_SEQUENCE_LOCKED") {
		release()
		t.Fatalf("second Gate lease was acquired: %v", err)
	}
	release()
	release, err = store.AcquireGateLease(ctx)
	if err != nil {
		t.Fatalf("released advisory lock file remained permanently stale: %v", err)
	}
	release()

	eventPath := filepath.Join(store.Directory(), "events.json")
	events, err := os.ReadFile(eventPath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := bytes.Replace(events, []byte(`"origin": "cli"`), []byte(`"origin": "clx"`), 1)
	if bytes.Equal(events, tampered) {
		t.Fatal("test could not locate event origin to tamper")
	}
	if err := os.WriteFile(eventPath, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(ctx); err == nil || !isErrorCode(err, "EVENT_HASH_INVALID") {
		t.Fatalf("event tampering was not detected: %v", err)
	}
}

func TestAdvisoryLockIsReleasedWhenHolderProcessExits(t *testing.T) {
	switch runtime.GOOS {
	case "darwin", "dragonfly", "freebsd", "linux", "netbsd", "openbsd":
	default:
		t.Skip("platform uses the conservative sentinel-lock fallback")
	}
	lockPath := filepath.Join(t.TempDir(), "crash-safe.lock")
	command := exec.Command(os.Args[0], "-test.run=^TestAdvisoryLockCrashHelper$")
	command.Env = append(os.Environ(), "ECP_TEST_CRASH_LOCK_PATH="+lockPath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("lock-holder subprocess failed: %v\n%s", err, output)
	}
	release, err := acquirePlatformFileLock(context.Background(), lockPath, time.Second, "LOCKED", "lock is held")
	if err != nil {
		t.Fatalf("process exit did not release the advisory lock: %v", err)
	}
	release()
}

func TestAdvisoryLockCrashHelper(t *testing.T) {
	path := os.Getenv("ECP_TEST_CRASH_LOCK_PATH")
	if path == "" {
		return
	}
	if _, err := acquirePlatformFileLock(context.Background(), path, time.Second, "LOCKED", "lock is held"); err != nil {
		os.Exit(91)
	}
	// Intentionally bypass defers to model an abruptly terminated Core. The OS
	// must release the advisory lock when this process exits.
	os.Exit(0)
}

func TestIgnoredControlPlaneStillChangesSourceFingerprint(t *testing.T) {
	repo := createTestRepository(t)
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".ecp/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	project, policy, gates := DefaultConfig("ignored-control", "prj-ignored-control", time.Now())
	if err := WriteInitialConfig(repo, project, policy, gates); err != nil {
		t.Fatal(err)
	}
	before, err := (Git{}).Snapshot(context.Background(), repo, policy)
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, ".ecp", "contracts", "boundaries.md"), "changed while ignored\n")
	after, err := (Git{}).Snapshot(context.Background(), repo, policy)
	if err != nil {
		t.Fatal(err)
	}
	if before.Fingerprint == after.Fingerprint {
		t.Fatal("ignored .ecp change was absent from source fingerprint")
	}
}

func TestStartChangeRefusesEmptyOrUnsupportedGatePlan(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	contextResult, err := service.InitProject(ctx, repo, "empty-gates")
	if err != nil {
		t.Fatal(err)
	}
	if contextResult.Assurance != string(ProjectAssuranceDisabled) || !hasDiagnostic(contextResult.Diagnostics, "ECP_DISABLED") {
		t.Fatalf("new Workspace was not disabled by default: %+v", contextResult)
	}
	status, err := service.ProjectStatus(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.EnableProject(ctx, repo, status.AuthorityID, status.WorkspaceID, status.ActivationToken, contextResult.CandidateConfig, contextResult.CandidateTruth, "owner", "attempt enable without a Gate"); err == nil || !isErrorCode(err, "NO_REQUIRED_GATES") {
		t.Fatalf("Workspace was enabled without a required Gate: %v", err)
	}
	status, err = service.ProjectStatus(ctx, repo)
	if err != nil || status.Enabled {
		t.Fatalf("failed enablement left a partially enabled Workspace: %+v err=%v", status, err)
	}
	network := passingGitGate()
	network.RequiresNetwork = true
	writeGates(t, repo, GatesConfig{SchemaVersion: SchemaVersion, Gates: []GateConfig{network}})
	if _, err := acceptTestPolicy(t, ctx, service, repo, digestBytes([]byte("invalid network candidate")), "owner", "network gate review"); err == nil || !isErrorCode(err, "NETWORK_GATE_UNSUPPORTED") {
		t.Fatalf("unsupported network Gate was accepted: %v", err)
	}

	riskRepo := createTestRepository(t)
	riskService := newTestService(t)
	moderateOnly := passingGitGate()
	moderateOnly.RequiredFor = []Risk{RiskModerate}
	bootstrapProjectWithGate(t, ctx, riskService, riskRepo, moderateOnly)
	bundle, err := LoadConfig(riskRepo)
	if err != nil {
		t.Fatal(err)
	}
	bundle.Policy.RiskRules = []PathRiskRule{{PathRoot: "src/sensitive", Risk: RiskHigh}}
	if err := writePrettyJSON(filepath.Join(riskRepo, ".ecp", "policy.json"), bundle.Policy, 0o644); err != nil {
		t.Fatal(err)
	}
	riskCandidate, err := LoadConfig(riskRepo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acceptTestPolicy(t, ctx, riskService, riskRepo, riskCandidate.Digest, "owner", "reviewed high-risk path rule"); err != nil {
		t.Fatal(err)
	}
	if _, err := startTestChange(t, ctx, riskService, riskRepo, StartChangeInput{
		Title: "Reachable high risk", Goal: "Must not enter an unrecoverable active state", Scope: []string{"src"},
		AcceptanceCriteria: []string{"risk-specific Gate exists"}, Risk: RiskModerate,
	}); err == nil || !isErrorCode(err, "NO_REQUIRED_GATES") {
		t.Fatalf("Change started even though an overlapping risk rule had no Gate: %v", err)
	}
	contextResult, err = riskService.Context(ctx, riskRepo)
	if err != nil || contextResult.ActiveChange != nil {
		t.Fatalf("failed risk preflight left an active Change: %+v err=%v", contextResult, err)
	}
	if got := effectiveRisk(
		Change{DeclaredRisk: RiskModerate},
		[]string{"deps/sub"},
		PolicyConfig{RiskRules: []PathRiskRule{{PathRoot: "deps/sub/security", Risk: RiskCritical}}},
	); got != RiskCritical {
		t.Fatalf("collapsed submodule path under-classified a descendant risk rule: %s", got)
	}
}

func TestGatePreflightCancellationAndAuthorityHistoryRecovery(t *testing.T) {
	ctx := context.Background()

	sensitiveRepo := createTestRepository(t)
	sensitiveService := newTestService(t)
	if _, err := sensitiveService.InitProject(ctx, sensitiveRepo, "sensitive-config"); err != nil {
		t.Fatal(err)
	}
	sensitiveGate := passingGitGate()
	sensitiveGate.InheritEnvironment = []string{"GITHUB_TOKEN"}
	writeGates(t, sensitiveRepo, GatesConfig{SchemaVersion: SchemaVersion, Gates: []GateConfig{sensitiveGate}})
	if _, err := acceptTestPolicy(t, ctx, sensitiveService, sensitiveRepo, digestBytes([]byte("invalid environment candidate")), "owner", "must reject impossible environment"); err == nil || !isErrorCode(err, "SENSITIVE_ENVIRONMENT_DENIED") {
		t.Fatalf("sensitive Gate environment reached an accepted epoch: %v", err)
	}

	preflightRepo := createTestRepository(t)
	preflightService := newTestService(t)
	preflightExecutable := filepath.Join(t.TempDir(), "preflight-gate")
	writeExecutable(t, preflightExecutable, "#!/bin/sh\nexit 0\n")
	preflightGate := gateWithCommand("preflight", []string{preflightExecutable})
	bootstrapProjectWithGate(t, ctx, preflightService, preflightRepo, preflightGate)
	if err := os.Remove(preflightExecutable); err != nil {
		t.Fatal(err)
	}
	if _, err := startTestChange(t, ctx, preflightService, preflightRepo, StartChangeInput{
		Title: "Unavailable Gate", Goal: "Do not enter ACTIVE", Scope: []string{"src"},
		AcceptanceCriteria: []string{"Gate context resolves"}, Risk: RiskModerate,
	}); err == nil || !isErrorCode(err, "GATE_EXECUTION_CONTEXT_UNAVAILABLE") {
		t.Fatalf("Change started after its required executable disappeared: %v", err)
	}
	preflightContext, err := preflightService.Context(ctx, preflightRepo)
	if err != nil || preflightContext.ActiveChange != nil {
		t.Fatalf("Gate preflight failure left an ACTIVE Change: %+v err=%v", preflightContext, err)
	}

	recoveryRepo := createTestRepository(t)
	recoveryService := newTestService(t)
	bootstrapProjectWithGate(t, ctx, recoveryService, recoveryRepo, passingGitGate())
	change, err := startTestChange(t, ctx, recoveryService, recoveryRepo, StartChangeInput{
		Title: "Recoverable cancellation", Goal: "Preserve authority history when Draft Config breaks", Scope: []string{"src"},
		AcceptanceCriteria: []string{"history remains queryable"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := runTestGates(t, ctx, recoveryService, recoveryRepo, change.ChangeID, nil)
	if err != nil || len(run.Evidence) != 1 {
		t.Fatalf("recovery precondition Gate failed: %+v err=%v", run, err)
	}
	if err := os.WriteFile(filepath.Join(recoveryRepo, ".ecp", "gates.json"), []byte("{\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	history, err := recoveryService.ListChanges(ctx, recoveryRepo)
	if err != nil || len(history) != 1 || history[0].State != ChangeActive {
		t.Fatalf("authority history depended on valid Draft Config: %+v err=%v", history, err)
	}
	evidence, err := recoveryService.ListEvidence(ctx, recoveryRepo, change.ChangeID)
	if err != nil || len(evidence.Evidence) != 1 {
		t.Fatalf("Evidence history depended on valid Draft Config: %+v err=%v", evidence, err)
	}
	cancellation, err := cancelTestChange(t, ctx, recoveryService, recoveryRepo, change.ChangeID, "owner", "Draft Config cannot be restored safely")
	if err != nil || cancellation.ChangeID != change.ChangeID {
		t.Fatalf("could not cancel through authority-only recovery path: %+v err=%v", cancellation, err)
	}
	history, err = recoveryService.ListChanges(ctx, recoveryRepo)
	if err != nil || len(history) != 1 || history[0].State != ChangeCancelled || history[0].Cancellation == nil {
		t.Fatalf("cancelled Change history is incomplete: %+v err=%v", history, err)
	}
}

func TestDefaultGateEnvironmentOmitsHomeAndRejectsCommonSecretCapabilities(t *testing.T) {
	project, policy, gates := DefaultConfig("environment-boundary", "prj-environment-boundary", time.Now())
	if containsStringValue(policy.InheritedEnvironment, "HOME") {
		t.Fatal("default Gate environment inherited HOME")
	}
	for _, name := range []string{"ANTHROPIC_API_KEY", "KUBECONFIG", "DOCKER_CONFIG", "SSH_AGENT_PID"} {
		gate := passingGitGate()
		gate.InheritEnvironment = []string{name}
		gates.Gates = []GateConfig{gate}
		if err := validateConfig(project, policy, gates); err == nil || !isErrorCode(err, "SENSITIVE_ENVIRONMENT_DENIED") {
			t.Fatalf("common secret capability %q was accepted: %v", name, err)
		}
	}
}

func TestActiveChangeMutationsRequireExactIdentity(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())

	first, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "First cancellation target", Goal: "Establish a stale cancellation target", Scope: []string{"src"},
		AcceptanceCriteria: []string{"cancellation is exact"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cancelTestChange(t, ctx, service, repo, first.ChangeID, "owner", "finish the first lifecycle"); err != nil {
		t.Fatal(err)
	}
	second, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Second cancellation target", Goal: "Remain active after a stale request", Scope: []string{"src"},
		AcceptanceCriteria: []string{"stale cancellation is rejected"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := service.Context(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	wrongAuthority := "auth-" + strings.Repeat("0", 32)
	if _, err := service.CancelChange(ctx, repo, wrongAuthority, target.WorkspaceID, second.ChangeID, "owner", "wrong authority target"); err == nil || !isErrorCode(err, "AUTHORITY_ID_MISMATCH") {
		t.Fatalf("cancellation accepted a different authority target: %v", err)
	}
	wrongWorkspace := "ws-" + strings.Repeat("0", 32)
	if _, err := service.CancelChange(ctx, repo, target.AuthorityID, wrongWorkspace, second.ChangeID, "owner", "wrong Workspace target"); err == nil || !isErrorCode(err, "WORKSPACE_ID_MISMATCH") {
		t.Fatalf("cancellation accepted a different Workspace target: %v", err)
	}
	if _, err := service.RunGates(ctx, repo, first.ChangeID, digestBytes([]byte("stale Gate plan")), nil); err == nil || !isErrorCode(err, "ACTIVE_CHANGE_MISMATCH") {
		t.Fatalf("stale Gate target was not rejected: %v", err)
	}
	staleSubject := digestBytes([]byte("stale active Change subject"))
	if _, err := service.CompleteChange(ctx, repo, first.ChangeID, staleSubject); err == nil || !isErrorCode(err, "ACTIVE_CHANGE_MISMATCH") {
		t.Fatalf("stale completion target was not rejected: %v", err)
	}
	if _, err := service.RecordAcknowledgement(ctx, repo, first.ChangeID, staleSubject, "owner", "stale acknowledgement target"); err == nil || !isErrorCode(err, "ACTIVE_CHANGE_MISMATCH") {
		t.Fatalf("stale acknowledgement target was not rejected: %v", err)
	}
	if _, err := cancelTestChange(t, ctx, service, repo, first.ChangeID, "owner", "stale request must not cancel the second Change"); err == nil || !isErrorCode(err, "ACTIVE_CHANGE_MISMATCH") {
		t.Fatalf("stale cancellation target was not rejected: %v", err)
	}
	evidence, err := service.ListEvidence(ctx, repo, second.ChangeID)
	if err != nil || len(evidence.Evidence) != 0 {
		t.Fatalf("stale Gate request produced Evidence for the new Change: %+v err=%v", evidence, err)
	}
	current, err := service.Context(ctx, repo)
	if err != nil || current.ActiveChange == nil || current.ActiveChange.ChangeID != second.ChangeID {
		t.Fatalf("stale cancellation changed the active Change: %+v err=%v", current.ActiveChange, err)
	}
	if _, err := cancelTestChange(t, ctx, service, repo, second.ChangeID, "owner", "clean up the exact second target"); err != nil {
		t.Fatal(err)
	}
}

func TestSingleActiveChangeAndScopePrefixBoundary(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	input := StartChangeInput{
		Title: "Scoped Change", Goal: "Only src is authorized", Scope: []string{"src"},
		AcceptanceCriteria: []string{"source check passes"}, Risk: RiskModerate,
	}
	change, err := startTestChange(t, ctx, service, repo, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := startTestChange(t, ctx, service, repo, input); err == nil || !isErrorCode(err, "ACTIVE_CHANGE_EXISTS") {
		t.Fatalf("second ACTIVE Change was created: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "src2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "src2", "outside.txt"), []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil); err != nil {
		t.Fatal(err)
	}
	verdict, err := service.Verdict(ctx, repo)
	if err != nil || verdict.Status != VerdictBlocked || !hasReasonPath(verdict, "OUT_OF_SCOPE", "src2/outside.txt") {
		t.Fatalf("src scope incorrectly included src2: %+v err=%v", verdict, err)
	}
}

func TestOutOfScopeRenameAndModeRemainVisible(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(repo, "docs", "outside.txt")
	if err := os.WriteFile(original, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "docs/outside.txt")
	runGit(t, repo, "commit", "-m", "add outside fixture")
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Rename boundary", Goal: "Detect both sides of an out-of-scope rename", Scope: []string{"src"},
		AcceptanceCriteria: []string{"scope blocks rename"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	renamed := filepath.Join(repo, "docs", "renamed.txt")
	if err := os.Rename(original, renamed); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(renamed, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil); err != nil {
		t.Fatal(err)
	}
	verdict, err := service.Verdict(ctx, repo)
	if err != nil || !hasReasonPath(verdict, "OUT_OF_SCOPE", "docs/outside.txt") || !hasReasonPath(verdict, "OUT_OF_SCOPE", "docs/renamed.txt") {
		t.Fatalf("rename endpoints were not both scope-checked: %+v err=%v", verdict, err)
	}
}

func TestConfigDriftStopsRemainingGateSequence(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	first := gateWithCommand("a-drift-config", []string{"sh", "-c", "printf drift >> .ecp/contracts/boundaries.md"})
	second := gateWithCommand("b-must-not-run", []string{"sh", "-c", "printf ran > second-ran"})
	bootstrapProjectWithGates(t, ctx, service, repo, []GateConfig{first, second})
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Config drift", Goal: "Stop a Gate sequence on policy drift", Scope: []string{"."},
		AcceptanceCriteria: []string{"second Gate does not run"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := service.PlanGates(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.RunGates(ctx, repo, change.ChangeID, plan.PlanDigest, nil)
	if err == nil || !isErrorCode(err, "CONFIG_CHANGED_DURING_GATE_SEQUENCE") || len(result.Evidence) != 1 {
		t.Fatalf("Gate sequence did not stop after config drift: result=%+v err=%v", result, err)
	}
	if _, err := os.Lstat(filepath.Join(repo, "second-ran")); !os.IsNotExist(err) {
		t.Fatalf("second Gate ran after config drift: %v", err)
	}
}

func TestGitPathWithTrailingSpacePreservesWorkspaceIdentity(t *testing.T) {
	parent := t.TempDir()
	repo := filepath.Join(parent, "repo ")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init", "-b", "main")
	root, err := (Git{}).DiscoverRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := canonicalExistingDirectory(repo)
	if err != nil {
		t.Fatal(err)
	}
	if root != canonical || !strings.HasSuffix(root, " ") {
		t.Fatalf("Git path protocol stripped a legal trailing space: root=%q canonical=%q", root, canonical)
	}
}

func gateWithCommand(id string, command []string) GateConfig {
	gate := passingGitGate()
	gate.ID = id
	gate.Command = command
	return gate
}

func bootstrapProjectWithGates(t *testing.T, ctx context.Context, service Service, repo string, gates []GateConfig) {
	t.Helper()
	if _, err := service.InitProject(ctx, repo, "test-project"); err != nil {
		t.Fatalf("init project: %v", err)
	}
	writeGates(t, repo, GatesConfig{SchemaVersion: SchemaVersion, Gates: gates})
	config, err := LoadConfig(repo)
	if err != nil {
		t.Fatalf("load candidate config: %v", err)
	}
	if _, err := acceptTestPolicy(t, ctx, service, repo, config.Digest, "test-owner", "accept test Gate definitions"); err != nil {
		t.Fatalf("accept policy: %v", err)
	}
	if _, err := enableTestProject(t, ctx, service, repo, config.Digest, "test-owner", "enable ECP for test Workspace"); err != nil {
		t.Fatalf("enable project: %v", err)
	}
}

func createCommitSeriesRepository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init", "-b", "main")
	runGit(t, repo, "config", "user.name", "ECP Test")
	runGit(t, repo, "config", "user.email", "ecp-test@example.invalid")
	path := filepath.Join(repo, "value.txt")
	for index, value := range []string{"A\n", "B\n", "C\n"} {
		if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
		runGit(t, repo, "add", "value.txt")
		runGit(t, repo, "commit", "-m", "commit-"+string(rune('A'+index)))
	}
	return repo
}

func gitOutput(t *testing.T, repo string, args ...string) string {
	t.Helper()
	command := append([]string{"-C", repo}, args...)
	cmd := exec.Command("git", command...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSuffix(strings.TrimSuffix(string(output), "\n"), "\r")
}

func readProjectConfig(t *testing.T, repo string) ProjectConfig {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repo, ".ecp", "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	var project ProjectConfig
	if err := json.Unmarshal(data, &project); err != nil {
		t.Fatal(err)
	}
	return project
}

func writeProjectConfig(t *testing.T, repo string, project ProjectConfig) {
	t.Helper()
	if err := writePrettyJSON(filepath.Join(repo, ".ecp", "project.json"), project, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func assertVerdictStatus(t *testing.T, service Service, repo string, status VerdictStatus) {
	t.Helper()
	verdict, err := service.Verdict(context.Background(), repo)
	if err != nil || verdict.Status != status {
		t.Fatalf("expected Verdict %s, got %+v err=%v", status, verdict, err)
	}
}

func assertVerdictReason(t *testing.T, service Service, repo string, status VerdictStatus, reason string) {
	t.Helper()
	verdict, err := service.Verdict(context.Background(), repo)
	if err != nil || verdict.Status != status || !hasReason(verdict, reason) {
		t.Fatalf("expected Verdict %s/%s, got %+v err=%v", status, reason, verdict, err)
	}
}

func hasDiagnostic(values []VerdictReason, code string) bool {
	for _, value := range values {
		if value.Code == code {
			return true
		}
	}
	return false
}
