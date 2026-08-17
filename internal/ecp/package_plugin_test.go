package ecp

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type packagedRuntimeEntry struct {
	Mode fs.FileMode
	Data string
}

func TestPackagePluginRuntimeSwapRecovery(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	packageScript, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "package-plugin.sh"))
	if err != nil {
		t.Fatal(err)
	}
	realMV, err := exec.LookPath("mv")
	if err != nil {
		t.Fatal(err)
	}

	for _, testCase := range []struct {
		name             string
		mvBehavior       string
		wantSuccess      bool
		wantOutputMarker string
	}{
		{name: "successful replacement", wantSuccess: true},
		{name: "install move failure restores previous runtime", mvBehavior: "fail-install", wantOutputMarker: "package-test: fail-install"},
		{name: "term after backup restores previous runtime", mvBehavior: "interrupt-after-backup", wantOutputMarker: "package-test: interrupt-after-backup"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixtureRoot, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			pluginRoot := createPackagePluginFixture(t, fixtureRoot, packageScript)
			runtimeRoot := filepath.Join(pluginRoot, "runtime")
			originalRuntime := snapshotPackagedRuntime(t, runtimeRoot)
			shimFixture, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			shimRoot := createPackagePluginTestShims(t, shimFixture)

			command := exec.Command("/bin/sh", filepath.Join(fixtureRoot, "scripts", "package-plugin.sh"))
			command.Dir = fixtureRoot
			command.Env = []string{
				"PATH=" + shimRoot + ":/usr/bin:/bin",
				"TMPDIR=" + filepath.Join(fixtureRoot, "tmp"),
				"GOFLAGS=-tags=ambient-must-not-leak",
				"GOENV=/ambient/go/env",
				"GOTOOLCHAIN=auto",
				"GOWORK=/ambient/go.work",
				"GOPROXY=https://ambient.invalid",
				"GOSUMDB=sum.ambient.invalid",
				"ECP_PACKAGE_TEST_REAL_MV=" + realMV,
				"ECP_PACKAGE_TEST_PLUGIN_ROOT=" + pluginRoot,
				"ECP_PACKAGE_TEST_MV_BEHAVIOR=" + testCase.mvBehavior,
			}
			output, runErr := command.CombinedOutput()
			if testCase.wantSuccess && runErr != nil {
				t.Fatalf("package command failed: %v\n%s", runErr, output)
			}
			if !testCase.wantSuccess && runErr == nil {
				t.Fatalf("package command unexpectedly succeeded:\n%s", output)
			}
			if testCase.wantOutputMarker != "" && !strings.Contains(string(output), testCase.wantOutputMarker) {
				t.Fatalf("package output does not prove the injected failure point %q:\n%s", testCase.wantOutputMarker, output)
			}

			stagingPaths, err := filepath.Glob(filepath.Join(pluginRoot, ".runtime-package.*"))
			if err != nil {
				t.Fatal(err)
			}
			if len(stagingPaths) != 0 {
				t.Fatalf("package staging residue remains: %v", stagingPaths)
			}

			if testCase.wantSuccess {
				assertPackagedRuntimeInstalled(t, runtimeRoot)
				return
			}
			restoredRuntime := snapshotPackagedRuntime(t, runtimeRoot)
			if !reflect.DeepEqual(restoredRuntime, originalRuntime) {
				t.Fatalf("previous runtime was not restored exactly\nwant: %#v\n got: %#v", originalRuntime, restoredRuntime)
			}
		})
	}
}

func TestPackagePluginRequiresCleanSourceUnlessDevelopmentOverride(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	packageScript, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "package-plugin.sh"))
	if err != nil {
		t.Fatal(err)
	}
	realMV, err := exec.LookPath("mv")
	if err != nil {
		t.Fatal(err)
	}
	fixtureRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pluginRoot := createPackagePluginFixture(t, fixtureRoot, packageScript)
	runtimeRoot := filepath.Join(pluginRoot, "runtime")
	originalRuntime := snapshotPackagedRuntime(t, runtimeRoot)
	writePackageTestFile(t, filepath.Join(fixtureRoot, "uncommitted-source.txt"), []byte("dirty source\n"), 0o644)
	shimFixture, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	shimRoot := createPackagePluginTestShims(t, shimFixture)
	baseEnvironment := []string{
		"PATH=" + shimRoot + ":/usr/bin:/bin",
		"TMPDIR=" + filepath.Join(fixtureRoot, "tmp"),
		"GOFLAGS=-tags=ambient-must-not-leak",
		"GOENV=/ambient/go/env",
		"GOTOOLCHAIN=auto",
		"GOWORK=/ambient/go.work",
		"GOPROXY=https://ambient.invalid",
		"GOSUMDB=sum.ambient.invalid",
		"ECP_PACKAGE_TEST_REAL_MV=" + realMV,
		"ECP_PACKAGE_TEST_PLUGIN_ROOT=" + pluginRoot,
		"ECP_PACKAGE_TEST_MV_BEHAVIOR=",
	}
	command := exec.Command("/bin/sh", filepath.Join(fixtureRoot, "scripts", "package-plugin.sh"))
	command.Dir = fixtureRoot
	command.Env = baseEnvironment
	output, runErr := command.CombinedOutput()
	if runErr == nil || !strings.Contains(string(output), "formal plugin packaging requires a clean source commit") {
		t.Fatalf("dirty formal package was not rejected before staging: err=%v\n%s", runErr, output)
	}
	if current := snapshotPackagedRuntime(t, runtimeRoot); !reflect.DeepEqual(current, originalRuntime) {
		t.Fatal("rejected dirty formal package changed the installed runtime")
	}

	command = exec.Command("/bin/sh", filepath.Join(fixtureRoot, "scripts", "package-plugin.sh"))
	command.Dir = fixtureRoot
	command.Env = append(append([]string(nil), baseEnvironment...), "ECP_PACKAGE_ALLOW_DIRTY=1")
	if output, runErr = command.CombinedOutput(); runErr != nil {
		t.Fatalf("explicit development package failed: %v\n%s", runErr, output)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(runtimeRoot, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		SourceClean bool `json:"source_clean"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SourceClean {
		t.Fatal("development dirty override incorrectly claimed a clean source")
	}
}

func createPackagePluginFixture(t *testing.T, root string, packageScript []byte) string {
	t.Helper()
	writePackageTestFile(t, filepath.Join(root, "scripts", "package-plugin.sh"), packageScript, 0o755)
	writePackageTestFile(t, filepath.Join(root, "go.mod"), []byte("module example.invalid/package-test\n\ngo 1.26\n"), 0o644)
	writePackageTestFile(t, filepath.Join(root, "cmd", "ecp", "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644)

	pluginRoot := filepath.Join(root, "plugins", "ecp-codex")
	writePackageTestFile(t, filepath.Join(pluginRoot, ".codex-plugin", "plugin.json"), []byte("{\n  \"name\": \"ecp-codex\",\n  \"version\": \"0.0.0-package-test\",\n  \"description\": \"package test fixture\"\n}\n"), 0o644)
	writePackageTestFile(t, filepath.Join(pluginRoot, "runtime", "manifest.json"), []byte("{\"old\":true}\n"), 0o644)
	writePackageTestFile(t, filepath.Join(pluginRoot, "runtime", "old-target", "ecp"), []byte("old-runtime-binary\n"), 0o755)
	writePackageTestFile(t, filepath.Join(pluginRoot, "runtime", "old-target", "ecp.sha256"), []byte("old-runtime-digest\n"), 0o644)
	if err := os.MkdirAll(filepath.Join(root, "tmp"), 0o755); err != nil {
		t.Fatal(err)
	}
	git := exec.Command("/usr/bin/git", "init", "--initial-branch=main")
	git.Dir = root
	if output, err := git.CombinedOutput(); err != nil {
		t.Fatalf("initialize package fixture Git repository: %v\n%s", err, output)
	}
	git = exec.Command("/usr/bin/git", "add", "--all")
	git.Dir = root
	if output, err := git.CombinedOutput(); err != nil {
		t.Fatalf("stage package fixture: %v\n%s", err, output)
	}
	git = exec.Command(
		"/usr/bin/git", "-c", "user.name=Package Test", "-c",
		"user.email=package-test@example.invalid", "commit", "-m", "fixture",
	)
	git.Dir = root
	if output, err := git.CombinedOutput(); err != nil {
		t.Fatalf("commit package fixture: %v\n%s", err, output)
	}
	return pluginRoot
}

func createPackagePluginTestShims(t *testing.T, root string) string {
	t.Helper()
	shimRoot := filepath.Join(root, "test-bin")
	writePackageTestFile(t, filepath.Join(shimRoot, "go"), []byte(`#!/bin/sh
set -eu

if [ "${1:-}" = "version" ]; then
  printf '%s\n' "go version go0.0.0-package-test test/arch"
  exit 0
fi

if [ "${PATH:-}" != "/usr/bin:/bin" ] || [ "${GOENV:-}" != "off" ] || [ "${GOTOOLCHAIN:-}" != "local" ] ||
   [ "${GOWORK:-}" != "off" ] || [ -n "${GOFLAGS:-}" ] || [ "${GOPROXY:-}" != "off" ] || [ "${GOSUMDB:-}" != "off" ]; then
  printf '%s\n' "package-test: ambient Go environment leaked into build" >&2
  exit 65
fi

output=
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    shift
    output=$1
  fi
  shift
done

if [ -z "$output" ]; then
  printf '%s\n' "package-test: fake go did not receive -o" >&2
  exit 64
fi
mkdir -p -- "${output%/*}"
printf '%s\n' "${GOOS}/${GOARCH}" > "$output"
`), 0o755)

	writePackageTestFile(t, filepath.Join(shimRoot, "mv"), []byte(`#!/bin/sh
set -eu

source_path=
destination_path=
for argument in "$@"; do
  if [ "$argument" = "--" ]; then
    continue
  fi
  source_path=$destination_path
  destination_path=$argument
done

case "$destination_path" in
  "$ECP_PACKAGE_TEST_PLUGIN_ROOT"/runtime)
    case "$source_path" in
      "$ECP_PACKAGE_TEST_PLUGIN_ROOT"/.runtime-package.*/runtime)
        if [ "$ECP_PACKAGE_TEST_MV_BEHAVIOR" = "fail-install" ]; then
          printf '%s\n' "package-test: fail-install" >&2
          exit 71
        fi
        ;;
      "$ECP_PACKAGE_TEST_PLUGIN_ROOT"/.runtime-package.*/previous-runtime)
        ;;
      *)
        printf '%s\n' "package-test: runtime install source is not a same-parent stage: $source_path" >&2
        exit 72
        ;;
    esac
    ;;
  "$ECP_PACKAGE_TEST_PLUGIN_ROOT"/.runtime-package.*/previous-runtime)
    "$ECP_PACKAGE_TEST_REAL_MV" "$@"
    if [ "$ECP_PACKAGE_TEST_MV_BEHAVIOR" = "interrupt-after-backup" ]; then
      parent_pid=$(/bin/ps -o ppid= -p $$ | tr -d '[:space:]')
      printf '%s\n' "package-test: interrupt-after-backup" >&2
      kill -TERM "$parent_pid"
    fi
    exit 0
    ;;
esac

exec "$ECP_PACKAGE_TEST_REAL_MV" "$@"
`), 0o755)
	return shimRoot
}

func writePackageTestFile(t *testing.T, path string, content []byte, mode fs.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, mode); err != nil {
		t.Fatal(err)
	}
}

func snapshotPackagedRuntime(t *testing.T, root string) map[string]packagedRuntimeEntry {
	t.Helper()
	snapshot := make(map[string]packagedRuntimeEntry)
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		item := packagedRuntimeEntry{Mode: info.Mode()}
		if info.Mode().IsRegular() {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			item.Data = string(content)
		}
		snapshot[relativePath] = item
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func assertPackagedRuntimeInstalled(t *testing.T, runtimeRoot string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(runtimeRoot, "old-target", "ecp")); !os.IsNotExist(err) {
		t.Fatalf("old runtime remains after successful replacement: %v", err)
	}
	for _, relativePath := range []string{
		"manifest.json",
		"darwin-arm64/ecp", "darwin-arm64/ecp.sha256",
		"darwin-amd64/ecp", "darwin-amd64/ecp.sha256",
		"linux-arm64/ecp", "linux-arm64/ecp.sha256",
		"linux-amd64/ecp", "linux-amd64/ecp.sha256",
	} {
		if _, err := os.Stat(filepath.Join(runtimeRoot, relativePath)); err != nil {
			t.Fatalf("packaged runtime is missing %s: %v", relativePath, err)
		}
	}
	manifestBytes, err := os.ReadFile(filepath.Join(runtimeRoot, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		SchemaVersion int    `json:"schema_version"`
		SourceCommit  string `json:"source_commit"`
		SourceClean   bool   `json:"source_clean"`
		Builder       struct {
			EnvironmentIsolation string `json:"environment_isolation"`
			GOENV                string `json:"goenv"`
			GOToolchain          string `json:"gotoolchain"`
			GOWork               string `json:"gowork"`
			GOFlags              string `json:"goflags"`
			GOProxy              string `json:"goproxy"`
			GOSumDB              string `json:"gosumdb"`
		} `json:"builder"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 3 || len(manifest.SourceCommit) != 40 || !manifest.SourceClean ||
		manifest.Builder.EnvironmentIsolation != "env-i" || manifest.Builder.GOENV != "off" || manifest.Builder.GOToolchain != "local" ||
		manifest.Builder.GOWork != "off" || manifest.Builder.GOFlags != "" || manifest.Builder.GOProxy != "off" || manifest.Builder.GOSumDB != "off" {
		t.Fatalf("clean formal package did not preserve source provenance: %+v", manifest)
	}
}
