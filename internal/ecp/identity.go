package ecp

import (
	"os"
	"path/filepath"
	"sync"
)

var (
	coreIdentityOnce sync.Once
	coreIdentity     string
	coreIdentityErr  error
)

func runtimeCoreIdentity() (string, error) {
	coreIdentityOnce.Do(func() {
		coreIdentity, coreIdentityErr = computeRuntimeCoreIdentity()
	})
	return coreIdentity, coreIdentityErr
}

func computeRuntimeCoreIdentity() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", newError(KindRuntime, "CORE_EXECUTABLE_UNAVAILABLE", "could not identify the running ECP Core executable", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", newError(KindRuntime, "CORE_EXECUTABLE_UNRESOLVED", "could not resolve the running ECP Core executable", err)
	}
	digest, err := digestFile(executable)
	if err != nil {
		return "", newError(KindRuntime, "CORE_EXECUTABLE_HASH_FAILED", "could not hash the running ECP Core executable", err)
	}
	return CoreVersion + "+" + digest, nil
}
