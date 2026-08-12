package ecp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode/utf8"
)

type Git struct{}

type sourceBudget struct {
	files int
	bytes int64
}

func (Git) DiscoverRoot(ctx context.Context, start string) (string, error) {
	canonical, err := canonicalExistingDirectory(start)
	if err != nil {
		return "", err
	}
	physicalRoot, err := discoverPhysicalGitRoot(canonical)
	if err != nil {
		return "", err
	}
	out, err := gitCommandForDiscovery(ctx, physicalRoot, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", newError(KindBlocked, "GIT_REPOSITORY_REQUIRED", "ECP v0.3 requires a Git repository", err)
	}
	root, err := parseGitPathOutput(out)
	if err != nil {
		return "", err
	}
	root, err = canonicalExistingDirectory(root)
	if err != nil {
		return "", newError(KindIntegrity, "GIT_ROOT_INVALID", "Git returned an invalid repository root", err)
	}
	if root != physicalRoot {
		return "", newError(KindIntegrity, "GIT_WORKTREE_MISMATCH", "Git configuration resolved a worktree different from the physical .git boundary", nil)
	}
	return root, nil
}

func (Git) WorkspaceID(ctx context.Context, root string) (string, error) {
	canonicalRoot, err := (Git{}).DiscoverRoot(ctx, root)
	if err != nil {
		return "", err
	}
	root = canonicalRoot
	out, err := gitCommand(ctx, root, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", newError(KindRuntime, "GIT_DIR_FAILED", "could not resolve the Git metadata directory", err)
	}
	gitDir, err := parseGitPathOutput(out)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(gitDir) {
		return "", newError(KindIntegrity, "GIT_DIR_NOT_ABSOLUTE", "Git returned a non-absolute metadata directory", nil)
	}
	gitDir, err = filepath.Abs(gitDir)
	if err != nil {
		return "", newError(KindRuntime, "GIT_DIR_FAILED", "could not canonicalize the Git metadata directory", err)
	}
	realGitDir, err := canonicalExistingDirectory(gitDir)
	if err != nil {
		return "", newError(KindIntegrity, "GIT_DIR_INVALID", "Git returned an invalid metadata directory", err)
	}
	gitDir = realGitDir
	sum := sha256.Sum256([]byte(filepath.Clean(root) + "\x00" + filepath.Clean(gitDir)))
	return "ws-" + hex.EncodeToString(sum[:16]), nil
}

func (Git) Snapshot(ctx context.Context, root string, policy PolicyConfig) (SourceSnapshot, error) {
	canonicalRoot, err := (Git{}).DiscoverRoot(ctx, root)
	if err != nil {
		return SourceSnapshot{}, err
	}
	first, err := (Git{}).snapshot(ctx, canonicalRoot, policy, 0, make(map[string]struct{}), &sourceBudget{})
	if err != nil {
		return SourceSnapshot{}, err
	}
	second, err := (Git{}).snapshot(ctx, canonicalRoot, policy, 0, make(map[string]struct{}), &sourceBudget{})
	if err != nil {
		return SourceSnapshot{}, err
	}
	if first.Fingerprint != second.Fingerprint {
		return SourceSnapshot{}, newError(KindConflict, "SOURCE_CHANGED_DURING_SNAPSHOT", "source changed while Core was constructing one stable fingerprint", nil)
	}
	return second, nil
}

func (Git) snapshot(ctx context.Context, root string, policy PolicyConfig, depth int, activeGitDirs map[string]struct{}, budget *sourceBudget) (SourceSnapshot, error) {
	if depth > 16 {
		return SourceSnapshot{}, newError(KindIntegrity, "SUBMODULE_DEPTH_LIMIT", "submodule nesting exceeds the v0.3 safety limit", nil)
	}
	gitExecutable, err := trustedGitExecutable()
	if err != nil {
		return SourceSnapshot{}, err
	}
	gitExecutableDigest, err := digestFile(gitExecutable)
	if err != nil {
		return SourceSnapshot{}, newError(KindRuntime, "GIT_EXECUTABLE_HASH_FAILED", "could not hash the trusted Git executable", err)
	}
	gitDirOut, err := gitCommand(ctx, root, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return SourceSnapshot{}, newError(KindIntegrity, "GIT_DIR_FAILED", "could not resolve Git metadata for source snapshot", err)
	}
	gitDir, err := parseGitPathOutput(gitDirOut)
	if err != nil {
		return SourceSnapshot{}, err
	}
	gitDir, err = canonicalExistingDirectory(gitDir)
	if err != nil {
		return SourceSnapshot{}, newError(KindIntegrity, "GIT_DIR_INVALID", "source snapshot Git metadata is invalid", err)
	}
	if _, exists := activeGitDirs[gitDir]; exists {
		return SourceSnapshot{}, newError(KindIntegrity, "SUBMODULE_CYCLE", "submodule metadata forms a recursive cycle", nil)
	}
	activeGitDirs[gitDir] = struct{}{}
	defer delete(activeGitDirs, gitDir)

	head, err := gitCommand(ctx, root, "rev-parse", "--verify", "HEAD")
	if err != nil {
		head = nil
	}
	headOID := strings.TrimSpace(string(head))
	branchOut, err := gitCommand(ctx, root, "symbolic-ref", "--short", "-q", "HEAD")
	branch := strings.TrimSpace(string(branchOut))
	if err != nil || branch == "" {
		if headOID == "" {
			branch = "UNBORN"
		} else {
			branch = "DETACHED"
		}
	}

	entries := make(map[string]*FileEntry)
	entryFor := func(path string) (*FileEntry, error) {
		if !utf8.ValidString(path) {
			return nil, newError(KindIntegrity, "INVALID_UTF8_GIT_PATH", "Git path is not valid UTF-8", nil)
		}
		normalized, err := normalizePathRoot(path)
		if err != nil || normalized == "." {
			return nil, newError(KindIntegrity, "INVALID_GIT_PATH", fmt.Sprintf("Git returned unsafe path %q", path), err)
		}
		entry := entries[normalized]
		if entry == nil {
			entry = &FileEntry{Path: normalized, WorktreeType: "absent"}
			entries[normalized] = entry
		}
		return entry, nil
	}

	if headOID != "" {
		out, err := gitCommand(ctx, root, "ls-tree", "-r", "-z", "--full-tree", "HEAD")
		if err != nil {
			return SourceSnapshot{}, newError(KindRuntime, "GIT_HEAD_TREE_FAILED", "could not inspect the HEAD tree", err)
		}
		for _, record := range splitNUL(out) {
			tab := bytes.IndexByte(record, '\t')
			if tab < 0 {
				return SourceSnapshot{}, newError(KindIntegrity, "GIT_TREE_PARSE_FAILED", "could not parse a Git tree record", nil)
			}
			meta := strings.Fields(string(record[:tab]))
			if len(meta) != 3 {
				return SourceSnapshot{}, newError(KindIntegrity, "GIT_TREE_PARSE_FAILED", "could not parse Git tree metadata", nil)
			}
			entry, err := entryFor(string(record[tab+1:]))
			if err != nil {
				return SourceSnapshot{}, err
			}
			entry.HeadMode = meta[0]
			entry.HeadObject = meta[2]
		}
	}

	indexOut, err := gitCommand(ctx, root, "ls-files", "--stage", "-z")
	if err != nil {
		return SourceSnapshot{}, newError(KindRuntime, "GIT_INDEX_FAILED", "could not inspect the Git index", err)
	}
	indexRecords := make(map[string][]string)
	for _, record := range splitNUL(indexOut) {
		tab := bytes.IndexByte(record, '\t')
		if tab < 0 {
			return SourceSnapshot{}, newError(KindIntegrity, "GIT_INDEX_PARSE_FAILED", "could not parse a Git index record", nil)
		}
		meta := strings.Fields(string(record[:tab]))
		if len(meta) != 3 {
			return SourceSnapshot{}, newError(KindIntegrity, "GIT_INDEX_PARSE_FAILED", "could not parse Git index metadata", nil)
		}
		entry, err := entryFor(string(record[tab+1:]))
		if err != nil {
			return SourceSnapshot{}, err
		}
		path := entry.Path
		indexRecords[path] = append(indexRecords[path], strings.Join(meta, "\x00"))
		if len(indexRecords[path]) == 1 {
			entry.IndexMode = meta[0]
			entry.IndexObject = meta[1]
			entry.IndexStage = meta[2]
		} else {
			entry.IndexMode = ""
			entry.IndexObject = ""
			entry.IndexStage = ""
		}
	}
	for path, records := range indexRecords {
		sort.Strings(records)
		entries[path].IndexState = digestBytes([]byte(strings.Join(records, "\x00")))
	}

	untrackedOut, err := gitCommand(ctx, root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return SourceSnapshot{}, newError(KindRuntime, "GIT_UNTRACKED_FAILED", "could not inspect untracked files", err)
	}
	for _, record := range splitNUL(untrackedOut) {
		if _, err := entryFor(string(record)); err != nil {
			return SourceSnapshot{}, err
		}
	}
	if err := addControlPlaneEntries(root, entryFor); err != nil {
		return SourceSnapshot{}, err
	}

	statusOut, err := gitCommand(ctx, root, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--no-renames", "--ignore-submodules=none")
	if err != nil {
		return SourceSnapshot{}, newError(KindRuntime, "GIT_STATUS_FAILED", "could not inspect Git status", err)
	}
	for _, record := range splitNUL(statusOut) {
		if len(record) < 4 || record[2] != ' ' {
			return SourceSnapshot{}, newError(KindIntegrity, "GIT_STATUS_PARSE_FAILED", "could not parse a Git status record", nil)
		}
		entry, err := entryFor(string(record[3:]))
		if err != nil {
			return SourceSnapshot{}, err
		}
		entry.Status = string(record[:2])
	}

	paths := make([]string, 0, len(entries))
	for path := range entries {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	budget.files += len(paths)
	if budget.files > policy.MaxSourceFiles {
		return SourceSnapshot{}, newError(KindIntegrity, "SOURCE_FILE_LIMIT", "source manifest exceeds max_source_files", nil)
	}

	manifest := make([]FileEntry, 0, len(paths))
	for _, path := range paths {
		entry := entries[path]
		indexHasGitlink := entry.IndexMode == "160000"
		for _, record := range indexRecords[path] {
			if strings.HasPrefix(record, "160000\x00") {
				indexHasGitlink = true
				break
			}
		}
		info, target, err := safeLstat(root, path)
		if indexHasGitlink {
			if err != nil || !info.IsDir() {
				return SourceSnapshot{}, newError(KindIntegrity, "SUBMODULE_UNAVAILABLE", fmt.Sprintf("indexed submodule %q is not an initialized directory", path), err)
			}
			entry.WorktreeMode = uint32(info.Mode().Perm())
			entry.WorktreeType = "submodule"
			submoduleRoot, err := (Git{}).DiscoverRoot(ctx, target)
			if err != nil || submoduleRoot != target {
				return SourceSnapshot{}, newError(KindIntegrity, "SUBMODULE_UNAVAILABLE", fmt.Sprintf("submodule %q is not an initialized, canonical Git worktree", path), err)
			}
			submodule, err := (Git{}).snapshot(ctx, submoduleRoot, policy, depth+1, activeGitDirs, budget)
			if err != nil {
				return SourceSnapshot{}, newError(KindIntegrity, "SUBMODULE_SNAPSHOT_FAILED", fmt.Sprintf("could not fingerprint submodule %q", path), err)
			}
			entry.ContentDigest = submodule.Fingerprint
			manifest = append(manifest, *entry)
			continue
		}
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				entry.WorktreeType = "absent"
				manifest = append(manifest, *entry)
				continue
			}
			return SourceSnapshot{}, err
		}
		entry.WorktreeMode = uint32(info.Mode().Perm())
		switch {
		case info.Mode().IsRegular():
			entry.WorktreeType = "regular"
			budget.bytes += info.Size()
			if budget.bytes > policy.MaxSourceBytes {
				return SourceSnapshot{}, newError(KindIntegrity, "SOURCE_BYTE_LIMIT", "source manifest exceeds max_source_bytes", nil)
			}
			digest, err := digestFile(target)
			if err != nil {
				return SourceSnapshot{}, newError(KindRuntime, "SOURCE_HASH_FAILED", fmt.Sprintf("could not hash %q", path), err)
			}
			entry.ContentDigest = digest
		case info.Mode()&os.ModeSymlink != 0:
			entry.WorktreeType = "symlink"
			link, err := os.Readlink(target)
			if err != nil {
				return SourceSnapshot{}, newError(KindRuntime, "SYMLINK_READ_FAILED", fmt.Sprintf("could not read symlink %q", path), err)
			}
			budget.bytes += int64(len(link))
			if budget.bytes > policy.MaxSourceBytes {
				return SourceSnapshot{}, newError(KindIntegrity, "SOURCE_BYTE_LIMIT", "source manifest exceeds max_source_bytes", nil)
			}
			entry.ContentDigest = digestBytes([]byte(link))
		default:
			return SourceSnapshot{}, newError(KindIntegrity, "UNSAFE_SOURCE_FILE", fmt.Sprintf("source path %q is not a regular file, symlink, or submodule", path), nil)
		}
		manifest = append(manifest, *entry)
	}

	unsigned := struct {
		SchemaVersion       int         `json:"schema_version"`
		Head                string      `json:"head"`
		Branch              string      `json:"branch"`
		GitExecutable       string      `json:"git_executable"`
		GitExecutableDigest string      `json:"git_executable_digest"`
		Entries             []FileEntry `json:"entries"`
	}{SchemaVersion: SchemaVersion, Head: headOID, Branch: branch, GitExecutable: gitExecutable, GitExecutableDigest: gitExecutableDigest, Entries: manifest}
	fingerprint, err := digestJSON(unsigned)
	if err != nil {
		return SourceSnapshot{}, newError(KindRuntime, "SOURCE_DIGEST_FAILED", "could not digest source manifest", err)
	}
	return SourceSnapshot{
		SchemaVersion:       SchemaVersion,
		Head:                headOID,
		Branch:              branch,
		GitExecutable:       gitExecutable,
		GitExecutableDigest: gitExecutableDigest,
		Fingerprint:         fingerprint,
		Entries:             manifest,
	}, nil
}

func TouchedPaths(baseline, current SourceSnapshot) []string {
	before := make(map[string]FileEntry, len(baseline.Entries))
	after := make(map[string]FileEntry, len(current.Entries))
	paths := make(map[string]struct{}, len(baseline.Entries)+len(current.Entries))
	for _, entry := range baseline.Entries {
		before[entry.Path] = entry
		paths[entry.Path] = struct{}{}
	}
	for _, entry := range current.Entries {
		after[entry.Path] = entry
		paths[entry.Path] = struct{}{}
	}
	touched := make([]string, 0)
	for path := range paths {
		// .ecp is reconciled through independent control-config and Project
		// Truth digests. Reporting it as product source would make every
		// legitimate truth update violate the Change's product-code scope.
		if containsPath(".ecp", path) {
			continue
		}
		a, aOK := before[path]
		b, bOK := after[path]
		if !aOK || !bOK || a != b {
			touched = append(touched, path)
		}
	}
	sort.Strings(touched)
	return touched
}

func gitCommand(ctx context.Context, root string, args ...string) ([]byte, error) {
	return runGitCommand(ctx, root, true, args...)
}

func gitCommandForDiscovery(ctx context.Context, root string, args ...string) ([]byte, error) {
	return runGitCommand(ctx, root, false, args...)
}

func runGitCommand(ctx context.Context, root string, pinWorktree bool, args ...string) ([]byte, error) {
	executable, err := trustedGitExecutable()
	if err != nil {
		return nil, err
	}
	base := []string{
		"-c", "core.hooksPath=" + os.DevNull,
		"-c", "core.fsmonitor=false",
		"-c", "core.bare=false",
		"-c", "diff.external=",
		"-c", "core.quotepath=false",
	}
	if pinWorktree {
		base = append(base, "-c", "core.worktree="+root)
	}
	base = append(base, "-C", root)
	cmd := exec.CommandContext(ctx, executable, append(base, args...)...)
	cmd.Env = gitEnvironment(executable)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func trustedGitExecutable() (string, error) {
	candidates := []string{"/usr/bin/git", "/bin/git"}
	if runtime.GOOS == "windows" {
		candidates = []string{
			`C:\Program Files\Git\cmd\git.exe`,
			`C:\Program Files\Git\bin\git.exe`,
			`C:\Program Files (x86)\Git\cmd\git.exe`,
			`C:\Windows\System32\git.exe`,
		}
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
			continue
		}
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil || !filepath.IsAbs(resolved) {
			continue
		}
		resolvedInfo, err := os.Stat(resolved)
		if err != nil || !resolvedInfo.Mode().IsRegular() || (runtime.GOOS != "windows" && resolvedInfo.Mode().Perm()&0o111 == 0) {
			continue
		}
		return resolved, nil
	}
	return "", newError(KindBlocked, "TRUSTED_GIT_UNAVAILABLE", "ECP could not find Git in a fixed system installation path; it will not resolve Core Git through ambient PATH", nil)
}

func discoverPhysicalGitRoot(start string) (string, error) {
	cursor := start
	for {
		marker := filepath.Join(cursor, ".git")
		info, err := os.Lstat(marker)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
				return "", newError(KindIntegrity, "UNSAFE_GIT_MARKER", ".git must be a real directory or regular gitdir file", nil)
			}
			return cursor, nil
		}
		if !os.IsNotExist(err) {
			return "", newError(KindRuntime, "GIT_MARKER_STAT_FAILED", "could not inspect the physical .git boundary", err)
		}
		parent := filepath.Dir(cursor)
		if parent == cursor {
			return "", newError(KindBlocked, "GIT_REPOSITORY_REQUIRED", "ECP v0.3 requires a physical Git worktree boundary", nil)
		}
		cursor = parent
	}
}

func parseGitPathOutput(output []byte) (string, error) {
	if bytes.ContainsRune(output, '\x00') || !utf8.Valid(output) {
		return "", newError(KindIntegrity, "INVALID_GIT_PATH_OUTPUT", "Git returned an invalid path", nil)
	}
	if bytes.HasSuffix(output, []byte("\r\n")) {
		output = output[:len(output)-2]
	} else if bytes.HasSuffix(output, []byte("\n")) {
		output = output[:len(output)-1]
	}
	if len(output) == 0 || bytes.ContainsAny(output, "\r\n") {
		return "", newError(KindIntegrity, "INVALID_GIT_PATH_OUTPUT", "Git returned an empty or multi-line path", nil)
	}
	return string(output), nil
}

func gitEnvironment(executable string) []string {
	environment := make([]string, 0, 10)
	for _, name := range []string{"TMPDIR", "LANG", "LC_ALL"} {
		if value, ok := os.LookupEnv(name); ok {
			environment = append(environment, name+"="+value)
		}
	}
	pathDirectories := []string{filepath.Dir(executable), "/usr/bin", "/bin", "/usr/sbin", "/sbin"}
	if runtime.GOOS == "windows" {
		gitRoot := filepath.Clean(filepath.Join(filepath.Dir(executable), "..", ".."))
		pathDirectories = []string{
			filepath.Dir(executable),
			filepath.Join(gitRoot, "bin"),
			filepath.Join(gitRoot, "usr", "bin"),
			filepath.Join(gitRoot, "mingw64", "bin"),
			`C:\Windows\System32`,
		}
	}
	seen := make(map[string]struct{}, len(pathDirectories))
	trustedPath := make([]string, 0, len(pathDirectories))
	for _, directory := range pathDirectories {
		if directory == "" || !filepath.IsAbs(directory) {
			continue
		}
		directory = filepath.Clean(directory)
		if _, exists := seen[directory]; exists {
			continue
		}
		seen[directory] = struct{}{}
		trustedPath = append(trustedPath, directory)
	}
	environment = append(environment, "PATH="+strings.Join(trustedPath, string(os.PathListSeparator)))
	return append(environment,
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_ATTR_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
	)
}

func addControlPlaneEntries(root string, entryFor func(string) (*FileEntry, error)) error {
	controlRoot := filepath.Join(root, ".ecp")
	if _, err := os.Lstat(controlRoot); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return newError(KindRuntime, "CONFIG_STAT_FAILED", "could not inspect .ecp for source fingerprinting", err)
	}
	return filepath.WalkDir(controlRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == controlRoot || entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		_, err = entryFor(filepath.ToSlash(relative))
		return err
	})
}

func splitNUL(data []byte) [][]byte {
	parts := bytes.Split(data, []byte{0})
	if len(parts) > 0 && len(parts[len(parts)-1]) == 0 {
		parts = parts[:len(parts)-1]
	}
	return parts
}

func safeLstat(root, relative string) (os.FileInfo, string, error) {
	normalized, err := normalizePathRoot(relative)
	if err != nil {
		return nil, "", err
	}
	parts := strings.Split(normalized, "/")
	current := root
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return nil, current, err
		}
		if i < len(parts)-1 && info.Mode()&os.ModeSymlink != 0 {
			return nil, current, newError(KindIntegrity, "SYMLINK_PARENT", fmt.Sprintf("path %q traverses a symlink parent", relative), nil)
		}
		if i == len(parts)-1 {
			return info, current, nil
		}
	}
	return nil, current, os.ErrNotExist
}

func digestFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	reader := bufio.NewReaderSize(file, 64*1024)
	if _, err := io.Copy(hash, reader); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
