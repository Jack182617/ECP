package ecp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"go/format"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRepositorySourceHygiene(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	textExtensions := map[string]struct{}{
		".go": {}, ".json": {}, ".md": {}, ".mod": {}, ".sh": {}, ".sha256": {}, ".yaml": {}, ".yml": {},
	}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if relative == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			t.Errorf("repository contains an unexpected symlink: %s", relative)
			return nil
		}
		if entry.Name() == ".DS_Store" {
			t.Errorf("repository contains Finder metadata: %s", relative)
			return nil
		}
		_, knownTextExtension := textExtensions[filepath.Ext(entry.Name())]
		knownTextPath := entry.Name() == ".gitignore" || filepath.ToSlash(relative) == "plugins/ecp-codex/scripts/ecp"
		if !knownTextExtension && !knownTextPath {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !utf8.Valid(contents) {
			t.Errorf("repository text file is not valid UTF-8: %s", relative)
			return nil
		}
		if bytes.Contains(contents, []byte{'\r'}) {
			t.Errorf("repository text file contains CR/CRLF line endings: %s", relative)
		}
		if len(contents) == 0 || contents[len(contents)-1] != '\n' {
			t.Errorf("repository text file lacks one final newline: %s", relative)
		}
		for lineNumber, line := range bytes.Split(contents, []byte{'\n'}) {
			if bytes.HasSuffix(line, []byte{' '}) || bytes.HasSuffix(line, []byte{'\t'}) {
				t.Errorf("repository text file has trailing whitespace at %s:%d", relative, lineNumber+1)
			}
		}
		if filepath.Ext(entry.Name()) == ".go" {
			formatted, err := format.Source(contents)
			if err != nil {
				t.Errorf("Go source cannot be formatted at %s: %v", relative, err)
			} else if !bytes.Equal(formatted, contents) {
				t.Errorf("Go source is not gofmt-formatted: %s", relative)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryProjectPackLoadsAsEstablishedTruth(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := LoadConfig(root)
	if err != nil {
		t.Fatalf("repository Project Pack is invalid: %v", err)
	}
	if bundle.Truth.Maturity != TruthMaturityEstablished {
		t.Fatalf("repository Project Truth maturity = %q, want %q", bundle.Truth.Maturity, TruthMaturityEstablished)
	}
	if len(bundle.Truth.Capabilities) == 0 || len(bundle.Truth.Invariants) == 0 || len(bundle.Truth.Components) == 0 {
		t.Fatalf("repository Project Truth lacks established facts: %+v", bundle.Truth)
	}
	if len(bundle.Truth.Unknowns) == 0 {
		t.Fatal("repository Project Truth must preserve material unproven boundaries as explicit unknowns")
	}
	unknowns := make(map[string]struct{}, len(bundle.Truth.Unknowns))
	for _, unknown := range bundle.Truth.Unknowns {
		unknowns[unknown.ID] = struct{}{}
	}
	for _, id := range []string{
		"independent-dual-track-pilot-unproven",
		"multi-year-value-unproven",
		"desktop-host-routing-not-proven",
		"desktop-plugin-lifecycle-not-proven",
		"trusted-plugin-distribution-not-proven",
	} {
		if _, ok := unknowns[id]; !ok {
			t.Fatalf("repository Project Truth collapsed or omitted independent unknown %q", id)
		}
	}
	coreGateFound := false
	for _, gate := range bundle.Gates.Gates {
		if gate.ID != "go-test-core" {
			continue
		}
		coreGateFound = true
		if gate.TimeoutSeconds < 600 {
			t.Fatalf("go-test-core timeout = %ds, want at least 600s measured headroom", gate.TimeoutSeconds)
		}
	}
	if !coreGateFound {
		t.Fatal("repository Project Pack lacks go-test-core Gate")
	}
	if bundle.Digest == bundle.TruthDigest {
		t.Fatal("control config and Project Truth must have independent digests")
	}
}

func TestDualTrackPilotProtocolAndReadinessClaimsStayAligned(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]string{
		filepath.Join(root, "docs", "real-project-pilot.md"): {
			"一个真正的新项目和一个已经持续开发的旧项目",
			"每个项目不少于八个真实 Change",
			"两个项目合计不少于二十个真实 Change",
			"Plugin 宿主预检的全部用例",
		},
		filepath.Join(root, "docs", "roadmap.md"): {
			"一个真正的新项目和一个持续开发的旧项目",
			"每个至少八个真实 Change，合计至少二十个",
			"fresh-task host-routing matrix",
		},
		filepath.Join(root, "docs", "verification-matrix.md"): {
			"Independent greenfield and established products, at least eight Changes each and twenty total",
			"Four-Skill host activation and output quality",
			"Fresh-task upgrade, uninstall, reinstall, and full four-Skill pickup",
		},
	}
	for path, fragments := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, fragment := range fragments {
			if !bytes.Contains(content, []byte(fragment)) {
				t.Fatalf("%s lost pilot-readiness boundary %q", path, fragment)
			}
		}
	}
}

func TestBundledPluginRuntimeIsCompleteAndPathIndependent(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	pluginRoot := filepath.Join(root, "plugins", "ecp-codex")
	manifestBytes, err := os.ReadFile(filepath.Join(pluginRoot, "runtime", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var runtimeManifest struct {
		SchemaVersion int    `json:"schema_version"`
		PluginName    string `json:"plugin_name"`
		PluginVersion string `json:"plugin_version"`
		Artifacts     []struct {
			OS        string `json:"os"`
			Arch      string `json:"arch"`
			Path      string `json:"path"`
			SHA256    string `json:"sha256"`
			SizeBytes int64  `json:"size_bytes"`
		} `json:"artifacts"`
	}
	if err := decodeStrictJSON(manifestBytes, &runtimeManifest); err != nil {
		t.Fatalf("runtime manifest is not strict JSON: %v", err)
	}
	pluginManifestBytes, err := os.ReadFile(filepath.Join(pluginRoot, ".codex-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pluginManifest struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(pluginManifestBytes, &pluginManifest); err != nil {
		t.Fatal(err)
	}
	if runtimeManifest.SchemaVersion != 1 || runtimeManifest.PluginName != pluginManifest.Name || runtimeManifest.PluginVersion != pluginManifest.Version {
		t.Fatalf("runtime and Plugin manifests are not version-bound: runtime=%+v plugin=%+v", runtimeManifest, pluginManifest)
	}

	expectedTargets := map[string]struct{}{
		"darwin/arm64": {}, "darwin/amd64": {}, "linux/arm64": {}, "linux/amd64": {},
	}
	for _, artifact := range runtimeManifest.Artifacts {
		key := artifact.OS + "/" + artifact.Arch
		if _, expected := expectedTargets[key]; !expected {
			t.Fatalf("unexpected or duplicated Plugin runtime target %q", key)
		}
		delete(expectedTargets, key)
		expectedPath := filepath.ToSlash(filepath.Join("runtime", artifact.OS+"-"+artifact.Arch, "ecp"))
		if artifact.Path != expectedPath {
			t.Fatalf("runtime path for %s = %q, want %q", key, artifact.Path, expectedPath)
		}
		binaryPath := filepath.Join(pluginRoot, filepath.FromSlash(artifact.Path))
		info, err := os.Lstat(binaryPath)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o111 == 0 || info.Size() != artifact.SizeBytes {
			t.Fatalf("runtime artifact %s is missing, unsafe, non-executable, or the wrong size: info=%v err=%v", key, info, err)
		}
		binary, err := os.ReadFile(binaryPath)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(binary)
		actualDigest := hex.EncodeToString(digest[:])
		if actualDigest != artifact.SHA256 {
			t.Fatalf("runtime artifact %s digest = %s, want %s", key, actualDigest, artifact.SHA256)
		}
		sidecar, err := os.ReadFile(binaryPath + ".sha256")
		if err != nil || strings.TrimSpace(string(sidecar)) != artifact.SHA256 {
			t.Fatalf("runtime artifact %s sidecar does not match its manifest: %q err=%v", key, sidecar, err)
		}
	}
	if len(expectedTargets) != 0 {
		t.Fatalf("Plugin runtime manifest is missing targets: %+v", expectedTargets)
	}

	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("host is intentionally outside the packaged runtime matrix")
	}
	if runtime.GOARCH != "arm64" && runtime.GOARCH != "amd64" {
		t.Skip("host architecture is intentionally outside the packaged runtime matrix")
	}
	launcherRelative := filepath.Join("scripts", "ecp")
	launcher := filepath.Join(pluginRoot, launcherRelative)
	assertLauncherVersion(t, launcher)

	installedPlugin := filepath.Join(t.TempDir(), "cache", "ecp-local", "ecp-codex", "local")
	if err := copyRegularTree(pluginRoot, installedPlugin); err != nil {
		t.Fatal(err)
	}
	installedLauncher := filepath.Join(installedPlugin, launcherRelative)
	assertLauncherVersion(t, installedLauncher)

	hostBinary := filepath.Join(installedPlugin, "runtime", runtime.GOOS+"-"+runtime.GOARCH, "ecp")
	file, err := os.OpenFile(hostBinary, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("corrupt")); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(installedLauncher, "version")
	command.Env = []string{"HOME=", "PATH=/nonexistent"}
	output, err := command.CombinedOutput()
	var exitError *exec.ExitError
	if err == nil || !strings.Contains(string(output), `"code":"ECP_RUNTIME_CHECKSUM_MISMATCH"`) || !errors.As(err, &exitError) || exitError.ExitCode() != 4 {
		t.Fatalf("corrupt installed runtime was not rejected: output=%s err=%v", output, err)
	}
}

func TestRepositoryMarketplaceTargetsBundledPlugin(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	marketplaceBytes, err := os.ReadFile(filepath.Join(root, ".agents", "plugins", "marketplace.json"))
	if err != nil {
		t.Fatal(err)
	}
	var marketplace struct {
		Name      string `json:"name"`
		Interface struct {
			DisplayName string `json:"displayName"`
		} `json:"interface"`
		Plugins []struct {
			Name   string `json:"name"`
			Source struct {
				Source string `json:"source"`
				Path   string `json:"path"`
			} `json:"source"`
			Policy struct {
				Installation   string `json:"installation"`
				Authentication string `json:"authentication"`
			} `json:"policy"`
			Category string `json:"category"`
		} `json:"plugins"`
	}
	if err := decodeStrictJSON(marketplaceBytes, &marketplace); err != nil {
		t.Fatalf("repo marketplace is not strict JSON: %v", err)
	}
	if marketplace.Name != "ecp-local" || marketplace.Interface.DisplayName != "ECP Local" || len(marketplace.Plugins) != 1 {
		t.Fatalf("repo marketplace identity is invalid: %+v", marketplace)
	}
	plugin := marketplace.Plugins[0]
	if plugin.Name != "ecp-codex" || plugin.Source.Source != "local" || plugin.Source.Path != "./plugins/ecp-codex" ||
		plugin.Policy.Installation != "AVAILABLE" || plugin.Policy.Authentication != "ON_INSTALL" || plugin.Category != "Productivity" {
		t.Fatalf("repo marketplace does not expose the intended local Plugin: %+v", plugin)
	}
	pluginRoot := filepath.Join(root, strings.TrimPrefix(plugin.Source.Path, "./"))
	if info, err := os.Stat(filepath.Join(pluginRoot, ".codex-plugin", "plugin.json")); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("repo marketplace Plugin source cannot be resolved: info=%v err=%v", info, err)
	}
}

func TestBundledSkillsEncodeFocusedAdapterSafetyBoundaries(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	pluginRoot := filepath.Join(root, "plugins", "ecp-codex")
	skillsRoot := filepath.Join(pluginRoot, "skills")
	entries, err := os.ReadDir(skillsRoot)
	if err != nil {
		t.Fatal(err)
	}
	wanted := map[string][]string{
		"ecp-check": {
			"Inspect ECP without enabling, disabling, or changing a project.",
			"Never infer current state from `.ecp`, Plugin installation, chat history",
			"also run `ecp version` through the same launcher",
			"label that fact `unverified`",
			"Never invoke project init/register/enable/disable",
		},
		"ecp-enable": {
			"only when the user explicitly asks to enable ECP",
			"Build or review the minimum truthful Project Pack",
			"Keep uncertainty as `Unknown`",
			"A name such as `test` or `check` is not safety evidence.",
			"Measure each exact command with cold and normal local caches",
			"Report success only when Core explicitly returns `enabled: true`",
		},
		"ecp-disable": {
			"only for an explicit whole-project disable request",
			"Disabled or unregistered mode is an idempotent success.",
			"Core atomically records it as CANCELLED before project disablement",
			"Never delete or edit `.ecp`, authority history, Evidence, source files",
		},
		"ecp-change": {
			"Do not ask the user to run ECP commands or copy IDs, hashes, or tokens.",
			"it is not an OS, shell, CI, or release enforcement boundary.",
			"Before any repository mutation, run only",
			"Do not initialize ECP, create a Change, or prompt the user to",
			"There is no task-level bypass",
			"Never set, synthesize, or infer a Verdict",
			"zero-silent-ambiguity guarantee",
			"--supersedes-change",
			"inferred_impact",
			"Do not run unrelated full suites merely for completeness.",
			"Local PASS does not mean defect-free, committed, pushed, deployed, published",
		},
	}
	seen := make(map[string]struct{}, len(wanted))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		required, ok := wanted[entry.Name()]
		if !ok {
			t.Fatalf("bundled Plugin contains unexpected Skill %q", entry.Name())
		}
		seen[entry.Name()] = struct{}{}
		skillPath := filepath.Join(skillsRoot, entry.Name(), "SKILL.md")
		content, err := os.ReadFile(skillPath)
		if err != nil {
			t.Fatal(err)
		}
		skill := string(content)
		normalizedSkill := strings.Join(strings.Fields(skill), " ")
		for _, shared := range []string{"../../references/cli-contract.md", "../../scripts/ecp", "Never use an ambient"} {
			if !strings.Contains(normalizedSkill, shared) {
				t.Fatalf("bundled Skill %s lost shared runtime contract %q", entry.Name(), shared)
			}
		}
		for _, fragment := range required {
			if !strings.Contains(normalizedSkill, fragment) {
				t.Fatalf("bundled Skill %s lost focused safety contract %q", entry.Name(), fragment)
			}
		}
		if strings.Contains(skill, "ask the user to copy the") || strings.Contains(skill, "local PASS proves release") {
			t.Fatalf("bundled Skill %s contains a forbidden user-protocol or release-proof instruction", entry.Name())
		}
		if info, err := os.Stat(filepath.Join(skillsRoot, entry.Name(), "agents", "openai.yaml")); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("bundled Skill %s lacks agents/openai.yaml: info=%v err=%v", entry.Name(), info, err)
		}
	}
	if len(seen) != len(wanted) {
		t.Fatalf("bundled Plugin Skill set mismatch: got=%v want=%v", seen, wanted)
	}
	for _, sharedPath := range []string{
		filepath.Join(pluginRoot, "scripts", "ecp"),
		filepath.Join(pluginRoot, "references", "cli-contract.md"),
		filepath.Join(pluginRoot, "references", "project-config.md"),
	} {
		if info, err := os.Stat(sharedPath); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("bundled Plugin shared resource is missing: path=%s info=%v err=%v", sharedPath, info, err)
		}
	}
	projectConfigBytes, err := os.ReadFile(filepath.Join(pluginRoot, "references", "project-config.md"))
	if err != nil {
		t.Fatal(err)
	}
	projectConfig := string(projectConfigBytes)
	for _, boundary := range []string{"measured fail-closed budget", "cold build/tool caches", "50% headroom", "depending on cache hits or retries"} {
		if !strings.Contains(projectConfig, boundary) {
			t.Fatalf("Plugin project-config reference lost Gate timeout boundary %q", boundary)
		}
	}
}

func TestPluginHostRoutingEvaluationInventory(t *testing.T) {
	type routingCase struct {
		ID                  string `json:"id"`
		Category            string `json:"category"`
		Prompt              string `json:"prompt"`
		InitialMode         string `json:"initial_mode"`
		ExpectedSkill       string `json:"expected_skill"`
		ExpectedBehavior    string `json:"expected_behavior"`
		RepositoryMutation  string `json:"repository_mutation"`
		ProjectModeMutation string `json:"project_mode_mutation"`
		MinimumRuns         int    `json:"minimum_runs"`
	}
	var inventory struct {
		SchemaVersion    int           `json:"schema_version"`
		ExecutionSurface string        `json:"execution_surface"`
		AcceptanceRule   string        `json:"acceptance_rule"`
		RecordFields     []string      `json:"record_fields"`
		Cases            []routingCase `json:"cases"`
	}

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	inventoryPath := filepath.Join(root, "docs", "plugin-host-evaluation-cases.json")
	content, err := os.ReadFile(inventoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := decodeStrictJSON(content, &inventory); err != nil {
		t.Fatalf("host-routing inventory is not valid JSON: %v", err)
	}
	if inventory.SchemaVersion != 1 || !strings.Contains(inventory.ExecutionSurface, "installed Plugin cache") || inventory.AcceptanceRule == "" {
		t.Fatalf("host-routing inventory lacks a versioned installed-host contract: %+v", inventory)
	}

	requiredRecordFields := []string{
		"case_id", "run_number", "task_id", "fixture_id", "initial_project_mode",
		"installed_plugin_version", "core_identity", "selected_skill",
		"observed_status_probe", "observed_repository_mutation",
		"observed_project_mode_mutation", "outcome", "notes",
	}
	recordFields := make(map[string]struct{}, len(inventory.RecordFields))
	for _, field := range inventory.RecordFields {
		if _, duplicate := recordFields[field]; duplicate {
			t.Fatalf("host-routing record field %q is duplicated", field)
		}
		recordFields[field] = struct{}{}
	}
	for _, field := range requiredRecordFields {
		if _, ok := recordFields[field]; !ok {
			t.Fatalf("host-routing records omit required field %q", field)
		}
	}

	allowedCategories := map[string]struct{}{
		"direct": {}, "indirect": {}, "incomplete": {}, "negative": {}, "edge": {},
	}
	allowedSkills := map[string]struct{}{
		"none": {}, "ecp-check": {}, "ecp-enable": {}, "ecp-disable": {}, "ecp-change": {},
	}
	allowedModes := map[string]struct{}{
		"any": {}, "disabled": {}, "enabled": {}, "enabled-blocked": {},
	}
	allowedRepositoryMutations := map[string]struct{}{
		"forbidden": {}, "enablement-only": {}, "governed-only": {}, "ordinary-after-disabled-status": {},
	}
	allowedProjectModeMutations := map[string]struct{}{
		"forbidden": {}, "explicit-enable-only": {}, "explicit-disable-only": {},
	}

	caseIDs := make(map[string]struct{}, len(inventory.Cases))
	categories := make(map[string]int, len(allowedCategories))
	modes := make(map[string]int, len(allowedModes))
	skillCategories := make(map[string]map[string]int)
	identifierPattern := regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	for _, item := range inventory.Cases {
		if !identifierPattern.MatchString(item.ID) {
			t.Fatalf("host-routing case has invalid id %q", item.ID)
		}
		if _, duplicate := caseIDs[item.ID]; duplicate {
			t.Fatalf("host-routing case id %q is duplicated", item.ID)
		}
		caseIDs[item.ID] = struct{}{}
		if _, ok := allowedCategories[item.Category]; !ok {
			t.Fatalf("host-routing case %s has unknown category %q", item.ID, item.Category)
		}
		if _, ok := allowedSkills[item.ExpectedSkill]; !ok {
			t.Fatalf("host-routing case %s has unknown expected Skill %q", item.ID, item.ExpectedSkill)
		}
		if _, ok := allowedModes[item.InitialMode]; !ok {
			t.Fatalf("host-routing case %s has unknown initial mode %q", item.ID, item.InitialMode)
		}
		if _, ok := allowedRepositoryMutations[item.RepositoryMutation]; !ok {
			t.Fatalf("host-routing case %s has unknown repository mutation rule %q", item.ID, item.RepositoryMutation)
		}
		if _, ok := allowedProjectModeMutations[item.ProjectModeMutation]; !ok {
			t.Fatalf("host-routing case %s has unknown project-mode mutation rule %q", item.ID, item.ProjectModeMutation)
		}
		if strings.TrimSpace(item.Prompt) != item.Prompt || item.Prompt == "" || strings.TrimSpace(item.ExpectedBehavior) != item.ExpectedBehavior || item.ExpectedBehavior == "" {
			t.Fatalf("host-routing case %s has empty or untrimmed prompt/behavior", item.ID)
		}
		if item.MinimumRuns < 2 {
			t.Fatalf("host-routing case %s requires only %d run(s); independent repetition is mandatory", item.ID, item.MinimumRuns)
		}
		if item.ExpectedSkill == "ecp-check" && (item.RepositoryMutation != "forbidden" || item.ProjectModeMutation != "forbidden") {
			t.Fatalf("read-only check case %s permits a mutation", item.ID)
		}
		if item.ExpectedSkill == "none" && (item.RepositoryMutation != "forbidden" || item.ProjectModeMutation != "forbidden") {
			t.Fatalf("negative no-Skill case %s permits a mutation", item.ID)
		}
		categories[item.Category]++
		modes[item.InitialMode]++
		if item.ExpectedSkill != "none" {
			if skillCategories[item.ExpectedSkill] == nil {
				skillCategories[item.ExpectedSkill] = make(map[string]int)
			}
			skillCategories[item.ExpectedSkill][item.Category]++
		}
	}

	for category := range allowedCategories {
		if categories[category] == 0 {
			t.Fatalf("host-routing inventory lacks category %q", category)
		}
	}
	for mode := range allowedModes {
		if modes[mode] == 0 {
			t.Fatalf("host-routing inventory lacks initial mode %q", mode)
		}
	}
	for _, skill := range []string{"ecp-check", "ecp-enable", "ecp-disable", "ecp-change"} {
		for _, category := range []string{"direct", "indirect"} {
			if skillCategories[skill][category] == 0 {
				t.Fatalf("host-routing inventory lacks a %s request for %s", category, skill)
			}
		}
	}
	if categories["negative"] < 4 {
		t.Fatalf("host-routing inventory has only %d negative cases; cross-Skill and no-Skill boundaries are under-specified", categories["negative"])
	}

	protocolBytes, err := os.ReadFile(filepath.Join(root, "docs", "plugin-host-evaluation.md"))
	if err != nil {
		t.Fatal(err)
	}
	protocol := string(protocolBytes)
	for _, boundary := range []string{
		"Never run enable/disable or governed-mutation evaluation against the ECP source",
		"Start a new Codex Desktop task with no prior ECP conversation",
		"Do not preserve full chat",
		"Any wrong route involving enable, disable, a repository write, a task-level",
		"every inventory case passes every",
	} {
		if !strings.Contains(protocol, boundary) {
			t.Fatalf("host-routing protocol lost boundary %q", boundary)
		}
	}
}

func TestVerificationMatrixCoversCanonicalScenariosAndExistingTests(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	specBytes, err := os.ReadFile(filepath.Join(root, "SPEC.md"))
	if err != nil {
		t.Fatal(err)
	}
	spec := string(specBytes)
	sectionStart := strings.Index(spec, "## 12. v0.3 验收场景")
	sectionEnd := strings.Index(spec, "## 13. 自举与保证边界")
	if sectionStart < 0 || sectionEnd <= sectionStart {
		t.Fatal("SPEC acceptance section boundaries are missing")
	}
	scenarioPattern := regexp.MustCompile(`(?m)^([1-9][0-9]*)\. `)
	scenarios := make(map[int]struct{})
	for _, match := range scenarioPattern.FindAllStringSubmatch(spec[sectionStart:sectionEnd], -1) {
		value, err := strconv.Atoi(match[1])
		if err != nil {
			t.Fatal(err)
		}
		if _, duplicate := scenarios[value]; duplicate {
			t.Fatalf("SPEC acceptance scenario %d is duplicated", value)
		}
		scenarios[value] = struct{}{}
	}

	matrixBytes, err := os.ReadFile(filepath.Join(root, "docs", "verification-matrix.md"))
	if err != nil {
		t.Fatal(err)
	}
	matrix := string(matrixBytes)
	matrixRowPattern := regexp.MustCompile(`(?m)^\| ([1-9][0-9]*) \|`)
	matrixRows := make(map[int]struct{})
	for _, match := range matrixRowPattern.FindAllStringSubmatch(matrix, -1) {
		value, err := strconv.Atoi(match[1])
		if err != nil {
			t.Fatal(err)
		}
		if _, duplicate := matrixRows[value]; duplicate {
			t.Fatalf("verification matrix scenario %d is duplicated", value)
		}
		matrixRows[value] = struct{}{}
	}
	if len(scenarios) != 75 || len(matrixRows) != len(scenarios) {
		t.Fatalf("acceptance traceability count mismatch: SPEC=%d matrix=%d", len(scenarios), len(matrixRows))
	}
	for scenario := range scenarios {
		if _, covered := matrixRows[scenario]; !covered {
			t.Fatalf("verification matrix does not cover SPEC scenario %d", scenario)
		}
	}
	for scenario := range matrixRows {
		if _, canonical := scenarios[scenario]; !canonical {
			t.Fatalf("verification matrix contains non-canonical scenario %d", scenario)
		}
	}

	declaredTests := make(map[string]struct{})
	testDeclarationPattern := regexp.MustCompile(`(?m)^func (Test[A-Za-z0-9_]+)\(`)
	err = filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range testDeclarationPattern.FindAllSubmatch(content, -1) {
			declaredTests[string(match[1])] = struct{}{}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	referencedTestPattern := regexp.MustCompile("`(Test[A-Za-z0-9_]+)`")
	for _, match := range referencedTestPattern.FindAllStringSubmatch(matrix, -1) {
		if _, exists := declaredTests[match[1]]; !exists {
			t.Fatalf("verification matrix references missing test %q", match[1])
		}
	}
}

func assertLauncherVersion(t *testing.T, launcher string) {
	t.Helper()
	command := exec.Command(launcher, "version")
	command.Env = []string{"HOME=", "PATH=/nonexistent"}
	output, err := command.Output()
	if err != nil {
		t.Fatalf("path-independent Plugin launcher failed: output=%s err=%v", output, err)
	}
	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			CoreVersion string `json:"core_version"`
		} `json:"result"`
	}
	if err := json.Unmarshal(output, &result); err != nil || !result.OK || result.Result.CoreVersion != CoreVersion {
		t.Fatalf("Plugin launcher returned the wrong Core version: output=%s err=%v", output, err)
	}
}

func copyRegularTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return newError(KindIntegrity, "PLUGIN_SOURCE_UNSAFE", "Plugin source contains a symlink", nil)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return newError(KindIntegrity, "PLUGIN_SOURCE_UNSAFE", "Plugin source contains a non-regular file", nil)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, info.Mode().Perm())
	})
}
