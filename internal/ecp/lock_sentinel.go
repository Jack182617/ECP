//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package ecp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

func acquirePlatformFileLock(ctx context.Context, path string, timeout time.Duration, code, message string) (func(), error) {
	deadline := time.Now().Add(timeout)
	for {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_, _ = fmt.Fprintf(file, "pid=%d\ncreated_at=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano))
			_ = file.Sync()
			_ = file.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, newError(KindRuntime, "LOCK_CREATE_FAILED", "could not create workspace lock", err)
		}
		if time.Now().After(deadline) {
			return nil, newError(KindConflict, code, message+"; this platform requires manual stale-lock recovery after a crash", nil)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}
