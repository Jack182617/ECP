package ecp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func randomID(prefix string, randomBytes int) (string, error) {
	b := make([]byte, randomBytes)
	if _, err := rand.Read(b); err != nil {
		return "", newError(KindRuntime, "RANDOM_FAILED", "could not generate a cryptographic identifier", err)
	}
	return prefix + "-" + hex.EncodeToString(b), nil
}

func digestJSON(value any) (string, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func digestBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func isSHA256Digest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func normalizePathRoot(value string) (string, error) {
	if value == "" {
		return "", newError(KindUsage, "EMPTY_PATH", "path root cannot be empty", nil)
	}
	if strings.ContainsRune(value, '\x00') {
		return "", newError(KindUsage, "NUL_PATH", "path root contains a NUL byte", nil)
	}
	if !utf8.ValidString(value) {
		return "", newError(KindUsage, "INVALID_UTF8_PATH", "path root must be valid UTF-8", nil)
	}
	value = filepath.ToSlash(value)
	if value == "." {
		return ".", nil
	}
	if filepath.IsAbs(filepath.FromSlash(value)) || strings.HasPrefix(value, "/") {
		return "", newError(KindUsage, "ABSOLUTE_PATH", "path root must be repository-relative", nil)
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	cleaned = strings.TrimSuffix(cleaned, "/")
	if cleaned == "." {
		return ".", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", newError(KindUsage, "PATH_TRAVERSAL", "path root escapes the repository", nil)
	}
	return cleaned, nil
}

func containsPath(root, candidate string) bool {
	if root == "." {
		return true
	}
	return candidate == root || strings.HasPrefix(candidate, root+"/")
}

func resolveWithinRoot(root, relative string) (string, error) {
	normalized, err := normalizePathRoot(relative)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(root, filepath.FromSlash(normalized))
	rel, err := filepath.Rel(root, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", newError(KindIntegrity, "PATH_OUTSIDE_ROOT", "resolved path escapes the repository root", err)
	}
	return candidate, nil
}

func canonicalExistingDirectory(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", newError(KindRuntime, "ABS_PATH_FAILED", "could not resolve absolute path", err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", newError(KindNotFound, "DIRECTORY_NOT_FOUND", "directory does not exist or cannot be resolved", err)
	}
	info, err := os.Stat(real)
	if err != nil {
		return "", newError(KindNotFound, "DIRECTORY_NOT_FOUND", "directory does not exist", err)
	}
	if !info.IsDir() {
		return "", newError(KindNotFound, "DIRECTORY_NOT_FOUND", "path is not a directory", nil)
	}
	return filepath.Clean(real), nil
}

func canonicalPotentialPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", newError(KindRuntime, "ABS_PATH_FAILED", "could not resolve absolute path", err)
	}
	abs = filepath.Clean(abs)
	missing := make([]string, 0)
	cursor := abs
	for {
		_, err := os.Lstat(cursor)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return "", newError(KindRuntime, "PATH_STAT_FAILED", "could not inspect path", err)
		}
		parent := filepath.Dir(cursor)
		if parent == cursor {
			return "", newError(KindRuntime, "PATH_ROOT_MISSING", "could not find an existing path ancestor", err)
		}
		missing = append(missing, filepath.Base(cursor))
		cursor = parent
	}
	real, err := filepath.EvalSymlinks(cursor)
	if err != nil {
		return "", newError(KindRuntime, "PATH_CANONICALIZE_FAILED", "could not canonicalize path", err)
	}
	for i := len(missing) - 1; i >= 0; i-- {
		real = filepath.Join(real, missing[i])
	}
	return filepath.Clean(real), nil
}

func ensureOutsideRoot(root, candidate string) error {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return newError(KindRuntime, "PATH_RELATIVE_FAILED", "could not compare repository and state paths", err)
	}
	if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return newError(KindIntegrity, "STATE_INSIDE_REPOSITORY", "ECP authority state must be outside the Git repository", nil)
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		return newError(KindRuntime, "ROOT_STAT_FAILED", "could not inspect the repository root", err)
	}
	// filepath.Rel is lexical and can disagree with physical identity on a
	// case-insensitive or Unicode-normalizing filesystem. Walk from the proposed
	// state path to its existing ancestors and compare inode/file identity so an
	// alternate spelling of a path inside the repository cannot cross the
	// authority boundary.
	for cursor := filepath.Clean(candidate); ; cursor = filepath.Dir(cursor) {
		info, statErr := os.Stat(cursor)
		if statErr == nil {
			if os.SameFile(rootInfo, info) {
				return newError(KindIntegrity, "STATE_INSIDE_REPOSITORY", "ECP authority state must be outside the physical Git repository", nil)
			}
		} else if !os.IsNotExist(statErr) {
			return newError(KindRuntime, "STATE_ANCESTOR_STAT_FAILED", "could not verify the physical authority-state boundary", statErr)
		}
		parent := filepath.Dir(cursor)
		if parent == cursor {
			break
		}
	}
	return nil
}

func validateIdentifier(value, label string) error {
	if value == "" || len(value) > 96 {
		return newError(KindIntegrity, "INVALID_IDENTIFIER", fmt.Sprintf("%s is empty or too long", label), nil)
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return newError(KindIntegrity, "INVALID_IDENTIFIER", fmt.Sprintf("%s contains unsupported characters", label), nil)
	}
	return nil
}
