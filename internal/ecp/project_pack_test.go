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
		knownTextPath := entry.Name() == ".gitignore" || filepath.ToSlash(relative) == "plugins/ecp-codex/skills/ecp-change/scripts/ecp"
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
	if bundle.Digest == bundle.TruthDigest {
		t.Fatal("control config and Project Truth must have independent digests")
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
	launcherRelative := filepath.Join("skills", "ecp-change", "scripts", "ecp")
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

func TestBundledSkillEncodesAdapterSafetyBoundaries(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(root, "plugins", "ecp-codex", "skills", "ecp-change", "SKILL.md")
	content, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	skill := string(content)
	normalizedSkill := strings.Join(strings.Fields(skill), " ")
	required := []string{
		"Do not ask the user to run ECP commands or copy IDs, hashes, or tokens.",
		"it is not an OS, shell, CI, or release enforcement boundary.",
		"Before any repository mutation, run only the bundled launcher as",
		"`enabled: false`: for an ordinary implementation request, end the ECP",
		"Do not initialize ECP, create a Change, or prompt the user to enable it.",
		"`enabled: true`: ECP governs every repository mutation until the project is",
		"there is no task-level bypass",
		"Never set, synthesize, or infer a Verdict",
		"This is a zero-silent-ambiguity guarantee",
		"--supersedes-change",
		"inferred_impact",
		"Do not run unrelated full suites merely for completeness",
		"Local PASS does not mean defect-free, committed, pushed, deployed, published, released, device-tested, production-tested",
	}
	for _, fragment := range required {
		if !strings.Contains(normalizedSkill, fragment) {
			t.Fatalf("bundled Skill lost required Adapter safety contract %q", fragment)
		}
	}
	if strings.Contains(skill, "ask the user to copy the") || strings.Contains(skill, "local PASS proves release") {
		t.Fatal("bundled Skill contains a forbidden user-protocol or release-proof instruction")
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
