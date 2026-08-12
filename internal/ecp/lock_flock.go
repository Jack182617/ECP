//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package ecp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

func acquirePlatformFileLock(ctx context.Context, path string, timeout time.Duration, code, message string) (func(), error) {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, newError(KindIntegrity, "UNSAFE_LOCK_FILE", "workspace lock path is not a regular file", nil)
		}
	} else if !os.IsNotExist(err) {
		return nil, newError(KindRuntime, "LOCK_STAT_FAILED", "could not inspect workspace lock", err)
	}

	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, newError(KindRuntime, "LOCK_CREATE_FAILED", "could not open workspace lock", err)
	}
	closeWithError := func(err error) (func(), error) {
		_ = file.Close()
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() {
		return closeWithError(newError(KindIntegrity, "UNSAFE_LOCK_FILE", "workspace lock file changed type while opening", err))
	}
	pathInfo, err := os.Lstat(path)
	if err != nil || pathInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(opened, pathInfo) {
		return closeWithError(newError(KindIntegrity, "UNSAFE_LOCK_FILE", "workspace lock path changed while opening", err))
	}
	if err := file.Chmod(0o600); err != nil {
		return closeWithError(newError(KindRuntime, "LOCK_CHMOD_FAILED", "could not secure workspace lock metadata", err))
	}

	deadline := time.Now().Add(timeout)
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			if err := file.Truncate(0); err != nil {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				return closeWithError(newError(KindRuntime, "LOCK_METADATA_FAILED", "could not truncate workspace lock metadata", err))
			}
			if _, err := file.Seek(0, 0); err != nil {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				return closeWithError(newError(KindRuntime, "LOCK_METADATA_FAILED", "could not seek workspace lock metadata", err))
			}
			if _, err := fmt.Fprintf(file, "pid=%d\ncreated_at=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				return closeWithError(newError(KindRuntime, "LOCK_METADATA_FAILED", "could not write workspace lock metadata", err))
			}
			if err := file.Sync(); err != nil {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				return closeWithError(newError(KindRuntime, "LOCK_METADATA_FAILED", "could not sync workspace lock metadata", err))
			}
			return func() {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				_ = file.Close()
			}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return closeWithError(newError(KindRuntime, "LOCK_ACQUIRE_FAILED", "could not acquire workspace advisory lock", err))
		}
		if time.Now().After(deadline) {
			return closeWithError(newError(KindConflict, code, message, nil))
		}
		select {
		case <-ctx.Done():
			return closeWithError(ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}
