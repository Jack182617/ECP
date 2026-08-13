package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ecp/internal/ecp"
)

func TestVersionUsesStableEnvelope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	application := CLI{Stdout: &stdout, Stderr: &stderr}
	if code := application.Run(context.Background(), []string{"version"}); code != 0 {
		t.Fatalf("version exit=%d stderr=%s", code, stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if result["schema_version"] != float64(1) || result["operation"] != "version" || result["ok"] != true {
		t.Fatalf("unexpected envelope: %#v", result)
	}
}

func TestSubcommandHelpIsAvailable(t *testing.T) {
	var stdout, stderr bytes.Buffer
	application := CLI{Stdout: &stdout, Stderr: &stderr}
	if code := application.Run(context.Background(), []string{"gate", "run", "--help"}); code != 0 {
		t.Fatalf("subcommand help exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "ecp gate run") || stderr.Len() != 0 {
		t.Fatalf("unexpected help output stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestAuthorityExportAndOfflineVerifyUseStableMachineContracts(t *testing.T) {
	ctx := context.Background()
	repo := createCLIRepository(t)
	service := ecp.Service{StateDir: filepath.Join(t.TempDir(), "state"), CoreIdentity: "cli-export-core"}
	if _, err := service.InitProject(ctx, repo, "cli-export"); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "bundle")
	var stdout, stderr bytes.Buffer
	application := CLI{Service: service, Stdout: &stdout, Stderr: &stderr}
	if code := application.Run(ctx, []string{"authority", "export", "--output", output, "--root", repo}); code != 0 {
		t.Fatalf("authority export exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var exportEnvelope struct {
		SchemaVersion int                       `json:"schema_version"`
		Operation     string                    `json:"operation"`
		OK            bool                      `json:"ok"`
		Result        ecp.AuthorityExportResult `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &exportEnvelope); err != nil || !exportEnvelope.OK || exportEnvelope.Operation != "authority.export" || exportEnvelope.Result.BundleDigest == "" || exportEnvelope.Result.Path == "" {
		t.Fatalf("invalid authority export envelope: %+v err=%v", exportEnvelope, err)
	}
	t.Cleanup(func() { makeCLIExportWritable(exportEnvelope.Result.Path) })

	stdout.Reset()
	stderr.Reset()
	if code := application.Run(ctx, []string{"authority", "verify", "--bundle", exportEnvelope.Result.Path}); code != 0 {
		t.Fatalf("authority verify exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var verifyEnvelope struct {
		SchemaVersion int                       `json:"schema_version"`
		Operation     string                    `json:"operation"`
		OK            bool                      `json:"ok"`
		Result        ecp.AuthorityVerifyResult `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &verifyEnvelope); err != nil || !verifyEnvelope.OK || verifyEnvelope.Operation != "authority.verify" || !verifyEnvelope.Result.Valid || verifyEnvelope.Result.BundleDigest != exportEnvelope.Result.BundleDigest {
		t.Fatalf("invalid authority verify envelope: %+v err=%v", verifyEnvelope, err)
	}
}

func TestAuthorityHealthUsesStructuredStatusExitCodes(t *testing.T) {
	ctx := context.Background()
	repo := createCLIRepository(t)
	stateDir := filepath.Join(t.TempDir(), "state")
	service := ecp.Service{StateDir: stateDir, CoreIdentity: "cli-health-core"}
	if _, err := service.InitProject(ctx, repo, "cli-health"); err != nil {
		t.Fatal(err)
	}
	status, err := service.ProjectStatus(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	authorityDir := filepath.Join(stateDir, "projects", status.ProjectID, "workspaces", status.WorkspaceID)
	var stdout, stderr bytes.Buffer
	application := CLI{Service: service, Stdout: &stdout, Stderr: &stderr}

	if code := application.Run(ctx, []string{"authority", "health", "--root", repo}); code != 0 {
		t.Fatalf("healthy authority exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var healthEnvelope struct {
		Operation string                    `json:"operation"`
		OK        bool                      `json:"ok"`
		Result    ecp.AuthorityHealthReport `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &healthEnvelope); err != nil || !healthEnvelope.OK || healthEnvelope.Operation != "authority.health" || healthEnvelope.Result.Status != ecp.AuthorityHealthHealthy {
		t.Fatalf("invalid healthy authority envelope: %+v err=%v", healthEnvelope, err)
	}

	unknown := filepath.Join(authorityDir, "unrecognized-private-file")
	if err := os.WriteFile(unknown, []byte("unreferenced"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := application.Run(ctx, []string{"authority", "health", "--root", repo}); code != 3 {
		t.Fatalf("attention authority exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	healthEnvelope = struct {
		Operation string                    `json:"operation"`
		OK        bool                      `json:"ok"`
		Result    ecp.AuthorityHealthReport `json:"result"`
	}{}
	if err := json.Unmarshal(stdout.Bytes(), &healthEnvelope); err != nil || !healthEnvelope.OK || healthEnvelope.Result.Status != ecp.AuthorityHealthAttention || stderr.Len() != 0 {
		t.Fatalf("attention must remain a successful structured result: %+v stderr=%s err=%v", healthEnvelope, stderr.String(), err)
	}
	if err := os.Remove(unknown); err != nil {
		t.Fatal(err)
	}

	config, err := ecp.LoadConfig(repo)
	if err != nil || len(config.TruthFiles) == 0 {
		t.Fatalf("could not resolve accepted truth fixture: %+v err=%v", config, err)
	}
	truthObject := filepath.Join(authorityDir, "truth-blobs", strings.TrimPrefix(config.TruthFiles[0].Digest, "sha256:"))
	if err := os.WriteFile(truthObject, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := application.Run(ctx, []string{"authority", "health", "--root", repo}); code != 4 {
		t.Fatalf("indeterminate authority exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	healthEnvelope = struct {
		Operation string                    `json:"operation"`
		OK        bool                      `json:"ok"`
		Result    ecp.AuthorityHealthReport `json:"result"`
	}{}
	if err := json.Unmarshal(stdout.Bytes(), &healthEnvelope); err != nil || !healthEnvelope.OK || healthEnvelope.Result.Status != ecp.AuthorityHealthIndeterminate || stderr.Len() != 0 {
		t.Fatalf("indeterminate object health must remain a successful structured result: %+v stderr=%s err=%v", healthEnvelope, stderr.String(), err)
	}
}

func TestProjectModeCommandsDefaultDisabledAndFailClosedOnDrift(t *testing.T) {
	ctx := context.Background()
	repo := createCLIRepository(t)
	stateDir := filepath.Join(t.TempDir(), "state")
	service := ecp.Service{StateDir: stateDir, CoreIdentity: "cli-project-mode-core"}
	var stdout, stderr bytes.Buffer
	application := CLI{Service: service, Stdout: &stdout, Stderr: &stderr}

	if code := application.Run(ctx, []string{"project", "status", "--root", repo}); code != 0 {
		t.Fatalf("default status exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var statusEnvelope struct {
		OK     bool              `json:"ok"`
		Result ecp.ProjectStatus `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &statusEnvelope); err != nil || !statusEnvelope.OK || statusEnvelope.Result.Enabled || statusEnvelope.Result.Assurance != ecp.ProjectAssuranceDisabled {
		t.Fatalf("invalid default status envelope: %+v err=%v", statusEnvelope, err)
	}
	if _, err := os.Lstat(filepath.Join(repo, ".ecp")); !os.IsNotExist(err) {
		t.Fatalf("project status created .ecp: %v", err)
	}
	if _, err := os.Lstat(stateDir); !os.IsNotExist(err) {
		t.Fatalf("project status created authority state: %v", err)
	}

	if _, err := service.InitProject(ctx, repo, "cli-project-mode"); err != nil {
		t.Fatal(err)
	}
	gate := ecp.GateConfig{
		ID: "passing", Description: "check Git diff whitespace", Command: []string{"git", "diff", "--check"}, WorkingDirectory: ".",
		TimeoutSeconds: 5, AllowedExitCodes: []int{0}, RequiredFor: []ecp.Risk{ecp.RiskLow, ecp.RiskModerate, ecp.RiskHigh, ecp.RiskCritical}, Environment: map[string]string{},
		InheritEnvironment: []string{}, MaxOutputBytes: 4096,
	}
	writeCLIGates(t, repo, ecp.GatesConfig{SchemaVersion: ecp.SchemaVersion, Gates: []ecp.GateConfig{gate}})
	candidate, err := ecp.LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acceptCLIPolicy(t, ctx, service, repo, candidate.Digest, "owner", "accept project-mode Gate"); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := application.Run(ctx, []string{"project", "status", "--root", repo}); code != 0 {
		t.Fatalf("prepared disabled status exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	statusEnvelope = struct {
		OK     bool              `json:"ok"`
		Result ecp.ProjectStatus `json:"result"`
	}{}
	if err := json.Unmarshal(stdout.Bytes(), &statusEnvelope); err != nil {
		t.Fatal(err)
	}
	disabled := statusEnvelope.Result
	stdout.Reset()
	stderr.Reset()
	if code := application.Run(ctx, []string{
		"project", "enable", "--authority", disabled.AuthorityID, "--workspace", disabled.WorkspaceID,
		"--activation-token", disabled.ActivationToken, "--config-digest", candidate.Digest,
		"--truth-digest", candidate.TruthDigest,
		"--actor", "owner", "--reason", "enable current project", "--root", repo,
	}); code != 0 {
		t.Fatalf("project enable exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &statusEnvelope); err != nil || !statusEnvelope.Result.Enabled || !statusEnvelope.Result.Operational {
		t.Fatalf("enable did not return operational status: %+v err=%v", statusEnvelope, err)
	}

	contractPath := filepath.Join(repo, ".ecp", "contracts", "boundaries.md")
	file, err := os.OpenFile(contractPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("policy drift\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := application.Run(ctx, []string{"project", "status", "--root", repo}); code != 3 {
		t.Fatalf("valid drift status exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &statusEnvelope); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 || !statusEnvelope.Result.Enabled || statusEnvelope.Result.Assurance != ecp.ProjectAssuranceBlocked {
		t.Fatalf("valid drift did not remain enabled+blocked: %+v stderr=%s", statusEnvelope, stderr.String())
	}

	if err := os.WriteFile(filepath.Join(repo, ".ecp", "gates.json"), []byte("{\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := application.Run(ctx, []string{"project", "status", "--root", repo}); code != 4 {
		t.Fatalf("malformed status exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &statusEnvelope); err != nil || !statusEnvelope.Result.Enabled || statusEnvelope.Result.Assurance != ecp.ProjectAssuranceIndeterminate {
		t.Fatalf("malformed config was not enabled+indeterminate: %+v err=%v", statusEnvelope, err)
	}
	broken := statusEnvelope.Result
	stdout.Reset()
	stderr.Reset()
	if code := application.Run(ctx, []string{
		"project", "disable", "--authority", broken.AuthorityID, "--workspace", broken.WorkspaceID,
		"--activation-token", broken.ActivationToken, "--actor", "owner", "--reason", "disable current project",
		"--root", repo,
	}); code != 0 {
		t.Fatalf("project disable exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &statusEnvelope); err != nil || statusEnvelope.Result.Enabled || statusEnvelope.Result.Assurance != ecp.ProjectAssuranceDisabled {
		t.Fatalf("disable did not return disabled status: %+v err=%v", statusEnvelope, err)
	}
}

func TestGateFailureAndIntegrityUseStableJSONExitCodes(t *testing.T) {
	ctx := context.Background()
	repo := createCLIRepository(t)
	service := ecp.Service{StateDir: filepath.Join(t.TempDir(), "state"), CoreIdentity: "cli-test-core"}
	if _, err := service.InitProject(ctx, repo, "cli-test"); err != nil {
		t.Fatal(err)
	}
	gate := ecp.GateConfig{
		ID: "failing", Description: "fail deterministically", Command: []string{"sh", "-c", `printf '\033]8;;https://example.invalid\007PASS approved\033]8;;\007'; exit 7`}, WorkingDirectory: ".",
		TimeoutSeconds: 5, AllowedExitCodes: []int{0}, RequiredFor: []ecp.Risk{ecp.RiskModerate}, Environment: map[string]string{},
		InheritEnvironment: []string{}, MaxOutputBytes: 4096,
	}
	writeCLIGates(t, repo, ecp.GatesConfig{SchemaVersion: ecp.SchemaVersion, Gates: []ecp.GateConfig{gate}})
	candidate, err := ecp.LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acceptCLIPolicy(t, ctx, service, repo, candidate.Digest, "owner", "accept failing CLI Gate"); err != nil {
		t.Fatal(err)
	}
	if _, err := enableCLIProject(t, ctx, service, repo, candidate.Digest, "owner", "enable CLI test project"); err != nil {
		t.Fatal(err)
	}
	change, err := startCLIChange(t, ctx, service, repo, ecp.StartChangeInput{
		Title: "CLI exit", Goal: "Prove stable exits", Scope: []string{"src"}, AcceptanceCriteria: []string{"failure is JSON"}, Risk: ecp.RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	application := CLI{Service: service, Stdout: &stdout, Stderr: &stderr}
	plan, err := service.PlanGates(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if code := application.Run(ctx, []string{"gate", "run", "--change", change.ChangeID, "--plan-digest", plan.PlanDigest, "--root", repo}); code != 3 {
		t.Fatalf("Gate failure exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var successEnvelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &successEnvelope); err != nil || successEnvelope["ok"] != true || stderr.Len() != 0 {
		t.Fatalf("Gate failure did not preserve result JSON: %#v err=%v stderr=%s", successEnvelope, err, stderr.String())
	}
	resultObject, ok := successEnvelope["result"].(map[string]any)
	if !ok {
		t.Fatalf("Gate result is not an object: %#v", successEnvelope)
	}
	evidenceItems, ok := resultObject["evidence"].([]any)
	if !ok || len(evidenceItems) != 1 {
		t.Fatalf("Gate result omitted Evidence metadata: %#v", resultObject)
	}
	evidenceObject, ok := evidenceItems[0].(map[string]any)
	if !ok {
		t.Fatalf("Evidence is not an object: %#v", evidenceItems[0])
	}
	if _, exists := evidenceObject["stdout"]; exists {
		t.Fatalf("raw untrusted stdout leaked into the CLI result envelope: %#v", evidenceObject)
	}
	if _, exists := evidenceObject["stderr"]; exists {
		t.Fatalf("raw untrusted stderr leaked into the CLI result envelope: %#v", evidenceObject)
	}

	config, err := ecp.LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, err := service.Git.WorkspaceID(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	store, err := ecp.NewStore(service.StateDir, config.Project.ProjectID, workspaceID, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	eventsPath := filepath.Join(store.Directory(), "events.json")
	events, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := bytes.Replace(events, []byte(`"origin": "cli"`), []byte(`"origin": "clx"`), 1)
	if bytes.Equal(events, tampered) {
		t.Fatal("could not tamper event fixture")
	}
	if err := os.WriteFile(eventsPath, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := application.Run(ctx, []string{"verdict", "--root", repo}); code != 4 {
		t.Fatalf("integrity failure exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("integrity error wrote stdout: %s", stdout.String())
	}
	var errorEnvelope map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &errorEnvelope); err != nil || errorEnvelope["ok"] != false {
		t.Fatalf("invalid integrity error JSON: %#v err=%v", errorEnvelope, err)
	}
	if _, exists := errorEnvelope["partial_result"]; exists {
		t.Fatalf("ordinary integrity error serialized a zero-value partial result: %#v", errorEnvelope)
	}
}

func TestProjectInspectBindsExactRegistrationDigest(t *testing.T) {
	ctx := context.Background()
	repo := createCLIRepository(t)
	project, policy, gates := ecp.DefaultConfig("cli-registration", "prj-cli-registration", time.Now())
	if err := ecp.WriteInitialConfig(repo, project, policy, gates); err != nil {
		t.Fatal(err)
	}
	service := ecp.Service{StateDir: filepath.Join(t.TempDir(), "state"), CoreIdentity: "cli-registration-core"}
	var stdout, stderr bytes.Buffer
	application := CLI{Service: service, Stdout: &stdout, Stderr: &stderr}
	if code := application.Run(ctx, []string{"project", "inspect", "--root", repo}); code != 0 {
		t.Fatalf("project inspect exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var envelope struct {
		Result ecp.ProjectInspection `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || envelope.Result.AuthorityID == "" || envelope.Result.WorkspaceID == "" || envelope.Result.CandidateConfigDigest == "" || envelope.Result.Registered {
		t.Fatalf("invalid project inspection: %+v err=%v", envelope.Result, err)
	}
	stdout.Reset()
	stderr.Reset()
	staleDigest := "sha256:" + strings.Repeat("0", 64)
	if code := application.Run(ctx, []string{"project", "register", "--authority", envelope.Result.AuthorityID, "--workspace", envelope.Result.WorkspaceID, "--config-digest", staleDigest, "--truth-digest", envelope.Result.CandidateTruthDigest, "--actor", "owner", "--reason", "stale candidate", "--root", repo}); code != 3 {
		t.Fatalf("stale registration exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), `"code": "CONFIG_DIGEST_MISMATCH"`) {
		t.Fatalf("stale registration did not report digest mismatch: %s", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := application.Run(ctx, []string{"project", "register", "--authority", envelope.Result.AuthorityID, "--workspace", envelope.Result.WorkspaceID, "--config-digest", envelope.Result.CandidateConfigDigest, "--truth-digest", envelope.Result.CandidateTruthDigest, "--actor", "owner", "--reason", "reviewed exact candidate", "--root", repo}); code != 0 {
		t.Fatalf("exact registration exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestCLIChangeStartAndTruthReconcileMachineContract(t *testing.T) {
	ctx := context.Background()
	repo := createCLIRepository(t)
	service := ecp.Service{StateDir: filepath.Join(t.TempDir(), "state"), CoreIdentity: "cli-truth-core"}
	if _, err := service.InitProject(ctx, repo, "cli-truth"); err != nil {
		t.Fatal(err)
	}
	gate := ecp.GateConfig{
		ID: "passing", Description: "pass deterministically", Command: []string{"git", "diff", "--check"}, WorkingDirectory: ".",
		TimeoutSeconds: 5, AllowedExitCodes: []int{0}, RequiredFor: []ecp.Risk{ecp.RiskModerate}, Environment: map[string]string{},
		InheritEnvironment: []string{}, MaxOutputBytes: 4096,
	}
	writeCLIGates(t, repo, ecp.GatesConfig{SchemaVersion: ecp.SchemaVersion, Gates: []ecp.GateConfig{gate}})
	candidate, err := ecp.LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acceptCLIPolicy(t, ctx, service, repo, candidate.Digest, "owner", "accept CLI truth Gate"); err != nil {
		t.Fatal(err)
	}
	if _, err := enableCLIProject(t, ctx, service, repo, candidate.Digest, "owner", "enable CLI truth project"); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	application := CLI{Service: service, Stdout: &stdout, Stderr: &stderr}
	if code := application.Run(ctx, []string{"policy", "get", "--root", repo}); code != 0 {
		t.Fatalf("CLI policy get exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var policyEnvelope struct {
		Result ecp.ControlConfigView `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &policyEnvelope); err != nil || policyEnvelope.Result.ConfigDigest != candidate.Digest || len(policyEnvelope.Result.Gates.Gates) != 1 {
		t.Fatalf("invalid CLI accepted policy result: %+v err=%v", policyEnvelope.Result, err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := application.Run(ctx, []string{"truth", "get", "--root", repo}); code != 0 {
		t.Fatalf("CLI truth get exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var truthEnvelope struct {
		Result ecp.ProjectTruthView `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &truthEnvelope); err != nil || truthEnvelope.Result.TruthDigest != candidate.TruthDigest || len(truthEnvelope.Result.Files) == 0 {
		t.Fatalf("invalid CLI accepted truth result: %+v err=%v", truthEnvelope.Result, err)
	}
	current, err := service.Context(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := application.Run(ctx, []string{
		"change", "start", "--root", repo,
		"--title", "CLI semantic contract", "--goal", "Exercise exact machine preconditions", "--scope", "src",
		"--acceptance", "semantic assessment is recorded", "--risk", "moderate", "--impact-unknown", "bootstrap truth is incomplete",
		"--expect-change", "Machine contract records a semantic assessment", "--expect-preserve", "Accepted Project Truth remains authoritative",
		"--requirement", `{"id":"semantic-contract","statement":"Record and preserve the declared semantic outcomes.","status":"DECIDED","verification":"REVIEW","rationale":"This CLI test reviews the bounded semantic contract directly.","decision_source":"CLI test fixture","covers":{"acceptance_criteria":["semantic assessment is recorded"],"expected_changes":["Machine contract records a semantic assessment"],"expected_preservations":["Accepted Project Truth remains authoritative"],"unknowns":["bootstrap truth is incomplete"]}}`,
		"--authority", current.AuthorityID, "--workspace", current.WorkspaceID, "--activation-token", current.ActivationToken,
		"--config-digest", current.CandidateConfig, "--truth-digest", current.CandidateTruth, "--source-fingerprint", current.Source.Fingerprint,
	}); code != 0 {
		t.Fatalf("CLI change start exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var startEnvelope struct {
		Result ecp.Change `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &startEnvelope); err != nil || startEnvelope.Result.ChangeID == "" || startEnvelope.Result.TruthDigest != current.CandidateTruth || len(startEnvelope.Result.Impact.ExpectedChanges) != 1 || len(startEnvelope.Result.Impact.ExpectedPreservations) != 1 {
		t.Fatalf("invalid CLI Change result: %+v err=%v", startEnvelope.Result, err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := application.Run(ctx, []string{"truth", "diff", "--root", repo}); code != 0 {
		t.Fatalf("CLI truth diff exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var diffEnvelope struct {
		Result ecp.TruthDiffReport `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &diffEnvelope); err != nil || diffEnvelope.Result.Changed || diffEnvelope.Result.ChangeID != startEnvelope.Result.ChangeID {
		t.Fatalf("invalid CLI truth diff: %+v err=%v", diffEnvelope.Result, err)
	}
	current, err = service.Context(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := application.Run(ctx, []string{
		"truth", "reconcile", "--root", repo,
		"--authority", current.AuthorityID, "--workspace", current.WorkspaceID, "--activation-token", current.ActivationToken,
		"--change", startEnvelope.Result.ChangeID, "--source-fingerprint", current.Source.Fingerprint,
		"--previous-truth-digest", diffEnvelope.Result.PreviousTruthDigest, "--candidate-truth-digest", diffEnvelope.Result.CandidateTruthDigest,
		"--behavior", "CHANGED", "--summary", "The CLI machine contract now records the expected semantic outcome while durable Project Truth remains accurate.", "--category", "behavior",
		"--requirement-result", `{"requirement_id":"semantic-contract","outcome":"VERIFIED","summary":"The CLI workflow reviewed the declared semantic contract."}`,
		"--actor", "owner", "--reason", "reviewed the exact source and truth digests",
	}); code != 0 {
		t.Fatalf("CLI truth reconcile exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var semanticEnvelope struct {
		Result ecp.SemanticAssessment `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &semanticEnvelope); err != nil || semanticEnvelope.Result.AssessmentID == "" || semanticEnvelope.Result.Behavior != ecp.SemanticBehaviorChanged || len(semanticEnvelope.Result.TruthDelta) != 0 {
		t.Fatalf("invalid CLI semantic assessment: %+v err=%v", semanticEnvelope.Result, err)
	}
}

func TestCompletedAndCancelledHistoryRemainQueryable(t *testing.T) {
	ctx := context.Background()
	repo := createCLIRepository(t)
	service := ecp.Service{StateDir: filepath.Join(t.TempDir(), "state"), CoreIdentity: "cli-history-core"}
	if _, err := service.InitProject(ctx, repo, "cli-history"); err != nil {
		t.Fatal(err)
	}
	gate := ecp.GateConfig{
		ID: "passing", Description: "pass deterministically", Command: []string{"git", "diff", "--check"}, WorkingDirectory: ".",
		TimeoutSeconds: 5, AllowedExitCodes: []int{0}, RequiredFor: []ecp.Risk{ecp.RiskModerate}, Environment: map[string]string{},
		InheritEnvironment: []string{}, MaxOutputBytes: 4096,
	}
	writeCLIGates(t, repo, ecp.GatesConfig{SchemaVersion: ecp.SchemaVersion, Gates: []ecp.GateConfig{gate}})
	candidate, err := ecp.LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acceptCLIPolicy(t, ctx, service, repo, candidate.Digest, "owner", "accept history Gate"); err != nil {
		t.Fatal(err)
	}
	if _, err := enableCLIProject(t, ctx, service, repo, candidate.Digest, "owner", "enable CLI history project"); err != nil {
		t.Fatal(err)
	}
	first, err := startCLIChange(t, ctx, service, repo, ecp.StartChangeInput{
		Title: "Completed history", Goal: "Persist final Verdict", Scope: []string{"src"},
		AcceptanceCriteria: []string{"Gate passes"}, Risk: ecp.RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := reconcileCLISemantic(t, ctx, service, repo, first.ChangeID); err != nil {
		t.Fatal(err)
	}
	plan, err := service.PlanGates(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RunGates(ctx, repo, first.ChangeID, plan.PlanDigest, nil); err != nil {
		t.Fatal(err)
	}
	finalVerdict, err := service.Verdict(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteChange(ctx, repo, first.ChangeID, finalVerdict.SubjectDigest); err != nil {
		t.Fatal(err)
	}
	second, err := startCLIChange(t, ctx, service, repo, ecp.StartChangeInput{
		Title: "Cancelled history", Goal: "Cancel despite broken Draft Config", Scope: []string{"src"},
		AcceptanceCriteria: []string{"Cancellation is audited"}, Risk: ecp.RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	cancelTarget, err := service.Context(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".ecp", "gates.json"), []byte("{\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	application := CLI{Service: service, Stdout: &stdout, Stderr: &stderr}
	if code := application.Run(ctx, []string{"evidence", "list", "--change", first.ChangeID, "--root", repo}); code != 0 {
		t.Fatalf("completed Evidence query exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || envelope["ok"] != true {
		t.Fatalf("invalid Evidence history envelope: %#v err=%v", envelope, err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := application.Run(ctx, []string{"change", "cancel", "--authority", cancelTarget.AuthorityID, "--workspace", cancelTarget.WorkspaceID, "--change", second.ChangeID, "--actor", "owner", "--reason", "broken Draft Config", "--root", repo}); code != 0 {
		t.Fatalf("cancel exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := application.Run(ctx, []string{"change", "list", "--root", repo}); code != 0 {
		t.Fatalf("Change history exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || envelope["ok"] != true || !strings.Contains(stdout.String(), `"state": "COMPLETED"`) || !strings.Contains(stdout.String(), `"state": "CANCELLED"`) {
		t.Fatalf("incomplete Change history envelope: %s err=%v", stdout.String(), err)
	}
	var historyEnvelope struct {
		Result []ecp.ChangeHistoryItem `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &historyEnvelope); err != nil || len(historyEnvelope.Result) != 2 || historyEnvelope.Result[0].Goal == "" || len(historyEnvelope.Result[0].AcceptanceCriteria) == 0 || len(historyEnvelope.Result[0].Impact.Unknowns) == 0 || len(historyEnvelope.Result[0].SemanticAssessments) == 0 {
		t.Fatalf("Change history lost durable contract or semantic context: %+v err=%v", historyEnvelope.Result, err)
	}
}

func TestEvidenceListEncodesEmptyArray(t *testing.T) {
	ctx := context.Background()
	repo := createCLIRepository(t)
	service := ecp.Service{StateDir: filepath.Join(t.TempDir(), "state"), CoreIdentity: "cli-empty-evidence-core"}
	if _, err := service.InitProject(ctx, repo, "cli-empty-evidence"); err != nil {
		t.Fatal(err)
	}
	gate := ecp.GateConfig{
		ID: "passing", Description: "pass deterministically", Command: []string{"git", "diff", "--check"}, WorkingDirectory: ".",
		TimeoutSeconds: 5, AllowedExitCodes: []int{0}, RequiredFor: []ecp.Risk{ecp.RiskModerate}, Environment: map[string]string{},
		InheritEnvironment: []string{}, MaxOutputBytes: 4096,
	}
	writeCLIGates(t, repo, ecp.GatesConfig{SchemaVersion: ecp.SchemaVersion, Gates: []ecp.GateConfig{gate}})
	candidate, err := ecp.LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acceptCLIPolicy(t, ctx, service, repo, candidate.Digest, "owner", "accept empty Evidence Gate"); err != nil {
		t.Fatal(err)
	}
	if _, err := enableCLIProject(t, ctx, service, repo, candidate.Digest, "owner", "enable empty Evidence project"); err != nil {
		t.Fatal(err)
	}
	change, err := startCLIChange(t, ctx, service, repo, ecp.StartChangeInput{
		Title: "Empty Evidence JSON", Goal: "Encode an empty array", Scope: []string{"src"},
		AcceptanceCriteria: []string{"Evidence JSON is an array"}, Risk: ecp.RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	application := CLI{Service: service, Stdout: &stdout, Stderr: &stderr}
	if code := application.Run(ctx, []string{"evidence", "list", "--change", change.ChangeID, "--root", repo}); code != 0 {
		t.Fatalf("empty Evidence query exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var envelope struct {
		Operation string `json:"operation"`
		OK        bool   `json:"ok"`
		Result    struct {
			Evidence json.RawMessage `json:"evidence"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid empty Evidence envelope: %v\n%s", err, stdout.String())
	}
	if !envelope.OK || envelope.Operation != "evidence.list" || !bytes.Equal(bytes.TrimSpace(envelope.Result.Evidence), []byte("[]")) {
		t.Fatalf("empty Evidence must encode as []: operation=%q ok=%t evidence=%s", envelope.Operation, envelope.OK, envelope.Result.Evidence)
	}
}

func TestGateSequenceErrorReturnsPartialEvidence(t *testing.T) {
	ctx := context.Background()
	repo := createCLIRepository(t)
	service := ecp.Service{StateDir: filepath.Join(t.TempDir(), "state"), CoreIdentity: "cli-partial-core"}
	if _, err := service.InitProject(ctx, repo, "cli-partial"); err != nil {
		t.Fatal(err)
	}
	gates := []ecp.GateConfig{
		{
			ID: "a-mutate-config", Description: "force sequence drift", Command: []string{"sh", "-c", "printf drift >> .ecp/contracts/boundaries.md"}, WorkingDirectory: ".",
			TimeoutSeconds: 5, AllowedExitCodes: []int{0}, RequiredFor: []ecp.Risk{ecp.RiskModerate}, Environment: map[string]string{}, InheritEnvironment: []string{}, MaxOutputBytes: 4096,
		},
		{
			ID: "z-never-runs", Description: "must be stopped", Command: []string{"git", "diff", "--check"}, WorkingDirectory: ".",
			TimeoutSeconds: 5, AllowedExitCodes: []int{0}, RequiredFor: []ecp.Risk{ecp.RiskModerate}, Environment: map[string]string{}, InheritEnvironment: []string{}, MaxOutputBytes: 4096,
		},
	}
	writeCLIGates(t, repo, ecp.GatesConfig{SchemaVersion: ecp.SchemaVersion, Gates: gates})
	candidate, err := ecp.LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acceptCLIPolicy(t, ctx, service, repo, candidate.Digest, "owner", "accept partial-result Gates"); err != nil {
		t.Fatal(err)
	}
	if _, err := enableCLIProject(t, ctx, service, repo, candidate.Digest, "owner", "enable CLI partial-result project"); err != nil {
		t.Fatal(err)
	}
	change, err := startCLIChange(t, ctx, service, repo, ecp.StartChangeInput{
		Title: "Partial result", Goal: "Preserve already-recorded Evidence", Scope: []string{"src"},
		AcceptanceCriteria: []string{"error names partial Evidence"}, Risk: ecp.RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	application := CLI{Service: service, Stdout: &stdout, Stderr: &stderr}
	plan, err := service.PlanGates(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if code := application.Run(ctx, []string{"gate", "run", "--change", change.ChangeID, "--plan-digest", plan.PlanDigest, "--root", repo}); code != 3 {
		t.Fatalf("partial Gate sequence exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("partial error unexpectedly wrote stdout: %s", stdout.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &result); err != nil || result["ok"] != false || result["partial_result"] == nil {
		t.Fatalf("partial Evidence was lost from error envelope: %#v err=%v", result, err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := application.Run(ctx, []string{"gate", "history", "--change", change.ChangeID, "--root", repo}); code != 0 {
		t.Fatalf("GateRun history exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var historyEnvelope struct {
		Operation string            `json:"operation"`
		OK        bool              `json:"ok"`
		Result    ecp.GateRunReport `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &historyEnvelope); err != nil || !historyEnvelope.OK || historyEnvelope.Operation != "gate.history" || len(historyEnvelope.Result.Runs) != 1 || historyEnvelope.Result.Runs[0].State != ecp.GateRunFailed || len(historyEnvelope.Result.Runs[0].EvidenceIDs) != 1 {
		t.Fatalf("CLI GateRun history lost the failed partial sequence: %+v err=%v", historyEnvelope, err)
	}
}

func createCLIRepository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runCLIGit(t, repo, "init", "-b", "main")
	runCLIGit(t, repo, "config", "user.name", "ECP CLI Test")
	runCLIGit(t, repo, "config", "user.email", "ecp-cli@example.invalid")
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "src", "app.txt"), []byte("baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCLIGit(t, repo, "add", ".")
	runCLIGit(t, repo, "commit", "-m", "baseline")
	return repo
}

func startCLIChange(t *testing.T, ctx context.Context, service ecp.Service, repo string, input ecp.StartChangeInput) (ecp.Change, error) {
	t.Helper()
	current, err := service.Context(ctx, repo)
	if err != nil {
		return ecp.Change{}, err
	}
	input.ExpectedWorkspace = current.WorkspaceID
	input.ExpectedAuthority = current.AuthorityID
	input.ExpectedActivation = current.ActivationToken
	input.ExpectedConfig = current.CandidateConfig
	input.ExpectedTruth = current.AcceptedTruth
	input.ExpectedSource = current.Source.Fingerprint
	if !input.Impact.ProjectPurpose && len(input.Impact.CapabilityIDs)+len(input.Impact.InvariantIDs)+len(input.Impact.ComponentIDs)+len(input.Impact.DecisionIDs)+len(input.Impact.ContractIDs)+len(input.Impact.UnknownIDs)+len(input.Impact.UserJourneys)+len(input.Impact.DataEffects)+len(input.Impact.OperationalEffects)+len(input.Impact.ExpectedChanges)+len(input.Impact.ExpectedPreservations)+len(input.Impact.Unknowns) == 0 {
		input.Impact.Unknowns = []string{"test impact intentionally not modeled"}
	}
	if len(input.Requirements) == 0 {
		input.Requirements = []ecp.ChangeRequirement{{
			ID: "cli-test-contract", Statement: "Exercise the exact CLI test contract.", Status: ecp.RequirementDecided,
			Verification: ecp.RequirementVerificationReview, Rationale: "The CLI test supplies focused assertions.", DecisionSource: "CLI test fixture",
			Covers: ecp.RequirementCoverage{
				AcceptanceCriteria:    append([]string(nil), input.AcceptanceCriteria...),
				UserJourneys:          append([]string(nil), input.Impact.UserJourneys...),
				DataEffects:           append([]string(nil), input.Impact.DataEffects...),
				OperationalEffects:    append([]string(nil), input.Impact.OperationalEffects...),
				ExpectedChanges:       append([]string(nil), input.Impact.ExpectedChanges...),
				ExpectedPreservations: append([]string(nil), input.Impact.ExpectedPreservations...),
				Unknowns:              append([]string(nil), input.Impact.Unknowns...),
			},
		}}
	}
	return service.StartChange(ctx, repo, input)
}

func reconcileCLISemantic(t *testing.T, ctx context.Context, service ecp.Service, repo, changeID string) error {
	t.Helper()
	current, err := service.Context(ctx, repo)
	if err != nil {
		return err
	}
	diff, err := service.TruthDiff(ctx, repo)
	if err != nil {
		return err
	}
	_, err = service.AssessSemantic(ctx, repo, ecp.SemanticAssessmentInput{
		ExpectedAuthority:      current.AuthorityID,
		ExpectedWorkspace:      current.WorkspaceID,
		ExpectedActivation:     current.ActivationToken,
		ExpectedChangeID:       changeID,
		ExpectedSource:         current.Source.Fingerprint,
		ExpectedPreviousTruth:  diff.PreviousTruthDigest,
		ExpectedCandidateTruth: diff.CandidateTruthDigest,
		Behavior:               ecp.SemanticBehaviorPreserved,
		Summary:                "CLI test confirms Project Truth remains valid.",
		Categories:             []string{"behavior"},
		RequirementAssessments: []ecp.RequirementAssessment{{RequirementID: "cli-test-contract", Outcome: ecp.RequirementOutcomeVerified, Summary: "Focused CLI test review reconciled this requirement."}},
		Actor:                  "test",
		Reason:                 "record semantic reconciliation before completion",
	})
	return err
}

func acceptCLIPolicy(t *testing.T, ctx context.Context, service ecp.Service, repo, configDigest, actor, reason string) (ecp.ConfigAcceptance, error) {
	t.Helper()
	target, err := service.Context(ctx, repo)
	if err != nil {
		return ecp.ConfigAcceptance{}, err
	}
	return service.AcceptPolicy(ctx, repo, target.AuthorityID, target.WorkspaceID, configDigest, actor, reason)
}

func enableCLIProject(t *testing.T, ctx context.Context, service ecp.Service, repo, configDigest, actor, reason string) (ecp.ProjectStatus, error) {
	t.Helper()
	target, err := service.ProjectStatus(ctx, repo)
	if err != nil {
		return ecp.ProjectStatus{}, err
	}
	return service.EnableProject(ctx, repo, target.AuthorityID, target.WorkspaceID, target.ActivationToken, configDigest, target.CandidateTruthDigest, actor, reason)
}

func runCLIGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func writeCLIGates(t *testing.T, repo string, gates ecp.GatesConfig) {
	t.Helper()
	data, err := json.MarshalIndent(gates, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(repo, ".ecp", "gates.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeCLIExportWritable(root string) {
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
}

func TestUsageErrorIsJSONAndExitTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	application := CLI{Stdout: &stdout, Stderr: &stderr}
	if code := application.Run(context.Background(), []string{"unknown"}); code != 2 {
		t.Fatalf("unknown command exit=%d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("usage error wrote stdout: %s", stdout.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &result); err != nil {
		t.Fatalf("invalid error JSON: %v\n%s", err, stderr.String())
	}
	if result["ok"] != false {
		t.Fatalf("unexpected error envelope: %#v", result)
	}
	stdout.Reset()
	stderr.Reset()
	fakeAuthority := "auth-" + strings.Repeat("0", 32)
	fakeWorkspace := "ws-" + strings.Repeat("0", 32)
	if code := application.Run(context.Background(), []string{"change", "cancel", "--authority", fakeAuthority, "--workspace", fakeWorkspace, "--actor", "owner", "--reason", "missing exact target"}); code != 2 {
		t.Fatalf("missing cancellation target exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), `"code": "CHANGE_ID_REQUIRED"`) {
		t.Fatalf("missing cancellation target did not return a stable usage error: %s", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := application.Run(context.Background(), []string{"policy", "accept", "--authority", fakeAuthority, "--workspace", fakeWorkspace, "--actor", "owner", "--reason", "missing exact config"}); code != 2 {
		t.Fatalf("missing policy digest exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), `"code": "CONFIG_DIGEST_REQUIRED"`) {
		t.Fatalf("missing policy digest did not return a stable usage error: %s", stderr.String())
	}
}
