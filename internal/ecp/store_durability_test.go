package ecp

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteReportsParentDirectorySyncFailure(t *testing.T) {
	target := filepath.Join(t.TempDir(), "state.json")
	injected := errors.New("injected directory sync failure")
	err := atomicWriteFileWithDirectorySync(target, []byte("durable candidate\n"), 0o600, func(string) error {
		return injected
	})
	if err == nil || !isErrorCode(err, "ATOMIC_DIRECTORY_SYNC_FAILED") || !errors.Is(err, injected) {
		t.Fatalf("directory sync failure was ignored: %v", err)
	}
	if content, readErr := os.ReadFile(target); readErr != nil || string(content) != "durable candidate\n" {
		t.Fatalf("test did not exercise the post-rename ambiguity boundary: content=%q err=%v", content, readErr)
	}
}
