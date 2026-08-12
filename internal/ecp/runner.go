package ecp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Execution struct {
	Command            []string
	WorkingDirectory   string
	ResolvedExecutable string
	ExecutableDigest   string
	EnvironmentNames   []string
	EnvironmentDigest  string
	StartedAt          time.Time
	FinishedAt         time.Time
	Duration           time.Duration
	ExitCode           int
	ProcessError       string
	TimedOut           bool
	ExitCodeAllowed    bool
	Stdout             []byte
	Stderr             []byte
	StdoutDigest       string
	StderrDigest       string
	StdoutBytes        int64
	StderrBytes        int64
	StdoutTruncated    bool
	StderrTruncated    bool
}

type Runner struct {
	Clock func() time.Time
}

type gateExecutionContext struct {
	WorkingDirectory   string
	ResolvedExecutable string
	ExecutableDigest   string
	Environment        []string
	EnvironmentNames   []string
	EnvironmentDigest  string
}

func (r Runner) Run(ctx context.Context, root string, policy PolicyConfig, gate GateConfig) (Execution, error) {
	if r.Clock == nil {
		r.Clock = time.Now
	}
	executionContext, err := resolveGateExecutionContext(root, policy, gate)
	if err != nil {
		return Execution{}, err
	}
	return r.runWithContext(ctx, policy, gate, executionContext)
}

func (r Runner) runWithContext(ctx context.Context, policy PolicyConfig, gate GateConfig, executionContext gateExecutionContext) (Execution, error) {
	if r.Clock == nil {
		r.Clock = time.Now
	}
	limit := gate.MaxOutputBytes
	if limit == 0 {
		limit = policy.MaxGateOutputBytes
	}
	stdout := newCappedDigestWriter(limit)
	stderr := newCappedDigestWriter(limit)

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(gate.TimeoutSeconds)*time.Second)
	defer cancel()
	command := exec.CommandContext(timeoutCtx, executionContext.ResolvedExecutable, gate.Command[1:]...)
	command.Dir = executionContext.WorkingDirectory
	command.Env = executionContext.Environment
	command.Stdout = stdout
	command.Stderr = stderr
	configureProcessCancellation(command)
	started := r.Clock().UTC()
	runErr := command.Run()
	finished := r.Clock().UTC()

	exitCode := 0
	processError := ""
	if runErr != nil {
		exitCode = -1
		var exitError *exec.ExitError
		if errorsAs(runErr, &exitError) {
			exitCode = exitError.ExitCode()
			processError = "process exited with a non-allowed status"
		} else {
			processError = runErr.Error()
		}
	}
	timedOut := timeoutCtx.Err() == context.DeadlineExceeded
	if timedOut {
		processError = "gate timed out"
	}
	allowed := !timedOut && containsExitCode(gate.AllowedExitCodes, exitCode)
	if allowed && runErr != nil {
		// A non-zero status can be explicitly allowed; it is then an observed
		// successful Gate result rather than a process startup failure.
		var exitError *exec.ExitError
		if errorsAs(runErr, &exitError) {
			processError = ""
		}
	}

	return Execution{
		Command:            append([]string(nil), gate.Command...),
		WorkingDirectory:   executionContext.WorkingDirectory,
		ResolvedExecutable: executionContext.ResolvedExecutable,
		ExecutableDigest:   executionContext.ExecutableDigest,
		EnvironmentNames:   executionContext.EnvironmentNames,
		EnvironmentDigest:  executionContext.EnvironmentDigest,
		StartedAt:          started,
		FinishedAt:         finished,
		Duration:           finished.Sub(started),
		ExitCode:           exitCode,
		ProcessError:       processError,
		TimedOut:           timedOut,
		ExitCodeAllowed:    allowed,
		Stdout:             stdout.Stored(),
		Stderr:             stderr.Stored(),
		StdoutDigest:       stdout.Digest(),
		StderrDigest:       stderr.Digest(),
		StdoutBytes:        stdout.Total(),
		StderrBytes:        stderr.Total(),
		StdoutTruncated:    stdout.Truncated(),
		StderrTruncated:    stderr.Truncated(),
	}, nil
}

func resolveGateExecutionContext(root string, policy PolicyConfig, gate GateConfig) (gateExecutionContext, error) {
	cwd, err := resolveGateWorkingDirectory(root, gate.WorkingDirectory)
	if err != nil {
		return gateExecutionContext{}, err
	}
	environment, names, environmentDigest, err := buildGateEnvironment(policy, gate)
	if err != nil {
		return gateExecutionContext{}, err
	}
	resolved, err := resolveExecutable(cwd, gate.Command[0], environment)
	if err != nil {
		return gateExecutionContext{}, err
	}
	executableDigest, err := digestFile(resolved)
	if err != nil {
		return gateExecutionContext{}, newError(KindRuntime, "GATE_EXECUTABLE_HASH_FAILED", "could not hash the resolved Gate executable", err)
	}
	return gateExecutionContext{
		WorkingDirectory:   cwd,
		ResolvedExecutable: resolved,
		ExecutableDigest:   executableDigest,
		Environment:        environment,
		EnvironmentNames:   names,
		EnvironmentDigest:  environmentDigest,
	}, nil
}

func resolveGateWorkingDirectory(root, relative string) (string, error) {
	candidate, err := resolveWithinRoot(root, relative)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", newError(KindIntegrity, "GATE_CWD_INVALID", "gate working directory does not exist or cannot be resolved", err)
	}
	rel, err := filepath.Rel(root, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", newError(KindIntegrity, "GATE_CWD_ESCAPE", "gate working directory escapes the repository through a symlink", err)
	}
	info, err := os.Stat(real)
	if err != nil || !info.IsDir() {
		return "", newError(KindIntegrity, "GATE_CWD_NOT_DIRECTORY", "gate working directory is not a directory", err)
	}
	return real, nil
}

func resolveExecutable(cwd, executable string, environment []string) (string, error) {
	var candidate string
	if strings.ContainsRune(executable, filepath.Separator) || strings.Contains(executable, "/") || strings.Contains(executable, "\\") {
		if filepath.IsAbs(executable) {
			candidate = executable
		} else {
			candidate = filepath.Join(cwd, filepath.FromSlash(executable))
		}
	} else {
		var err error
		candidate, err = lookPathInEnvironment(executable, environment)
		if err != nil {
			return "", newError(KindBlocked, "GATE_EXECUTABLE_NOT_FOUND", fmt.Sprintf("gate executable %q was not found", executable), err)
		}
	}
	candidate, err := filepath.Abs(candidate)
	if err != nil {
		return "", newError(KindRuntime, "GATE_EXECUTABLE_PATH_FAILED", "could not resolve gate executable", err)
	}
	real, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", newError(KindBlocked, "GATE_EXECUTABLE_UNRESOLVED", "gate executable could not be resolved", err)
	}
	info, err := os.Stat(real)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", newError(KindBlocked, "GATE_EXECUTABLE_UNSAFE", "gate executable is not an executable regular file", err)
	}
	return real, nil
}

func lookPathInEnvironment(executable string, environment []string) (string, error) {
	pathValue := ""
	for _, pair := range environment {
		if strings.HasPrefix(pair, "PATH=") {
			pathValue = strings.TrimPrefix(pair, "PATH=")
			break
		}
	}
	if pathValue == "" {
		return "", fmt.Errorf("PATH is absent from the Gate environment")
	}
	for _, directory := range filepath.SplitList(pathValue) {
		if directory == "" || !filepath.IsAbs(directory) {
			continue
		}
		candidate := filepath.Join(directory, executable)
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", exec.ErrNotFound
}

func buildGateEnvironment(policy PolicyConfig, gate GateConfig) ([]string, []string, string, error) {
	allowed := make(map[string]struct{})
	for _, name := range append(append([]string(nil), policy.InheritedEnvironment...), gate.InheritEnvironment...) {
		if err := validateEnvironmentName(name); err != nil {
			return nil, nil, "", err
		}
		if isSensitiveEnvironmentName(name) {
			return nil, nil, "", newError(KindBlocked, "SENSITIVE_ENVIRONMENT_DENIED", fmt.Sprintf("environment variable %q is denied by the v0.3 core", name), nil)
		}
		allowed[name] = struct{}{}
	}
	values := make(map[string]string)
	for name := range allowed {
		if value, ok := os.LookupEnv(name); ok {
			values[name] = value
		}
	}
	for name, value := range gate.Environment {
		if err := validateEnvironmentName(name); err != nil {
			return nil, nil, "", err
		}
		if isSensitiveEnvironmentName(name) {
			return nil, nil, "", newError(KindBlocked, "SENSITIVE_ENVIRONMENT_DENIED", fmt.Sprintf("explicit environment variable %q is denied by the v0.3 core", name), nil)
		}
		if strings.ContainsRune(value, '\x00') {
			return nil, nil, "", newError(KindIntegrity, "INVALID_ENVIRONMENT_VALUE", fmt.Sprintf("environment variable %q contains NUL", name), nil)
		}
		values[name] = value
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	environment := make([]string, 0, len(names))
	var digestInput bytes.Buffer
	for _, name := range names {
		pair := name + "=" + values[name]
		environment = append(environment, pair)
		digestInput.WriteString(name)
		digestInput.WriteByte(0)
		digestInput.WriteString(values[name])
		digestInput.WriteByte(0)
	}
	return environment, names, digestBytes(digestInput.Bytes()), nil
}

func validateEnvironmentName(name string) error {
	if name == "" {
		return newError(KindIntegrity, "INVALID_ENVIRONMENT_NAME", "environment variable name is empty", nil)
	}
	for i, r := range name {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return newError(KindIntegrity, "INVALID_ENVIRONMENT_NAME", fmt.Sprintf("environment variable name %q is invalid", name), nil)
	}
	return nil
}

func isSensitiveEnvironmentName(name string) bool {
	upper := strings.ToUpper(name)
	if upper == "SSH_AUTH_SOCK" || upper == "SSH_AGENT_PID" || upper == "GPG_AGENT_INFO" || upper == "DOCKER_AUTH_CONFIG" || upper == "DOCKER_CONFIG" || upper == "KUBECONFIG" || upper == "XDG_CONFIG_HOME" || upper == "XDG_RUNTIME_DIR" {
		return true
	}
	for _, fragment := range []string{"TOKEN", "SECRET", "PASSWORD", "PASSWD", "CREDENTIAL", "PRIVATE_KEY", "API_KEY", "ACCESS_KEY"} {
		if strings.Contains(upper, fragment) {
			return true
		}
	}
	for _, prefix := range []string{"AWS_", "AZURE_", "GOOGLE_", "GITHUB_", "OPENAI_API_"} {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	return false
}

func containsExitCode(allowed []int, value int) bool {
	for _, candidate := range allowed {
		if candidate == value {
			return true
		}
	}
	return false
}

type cappedDigestWriter struct {
	limit     int64
	total     int64
	buffer    bytes.Buffer
	hash      hash.Hash
	truncated bool
}

func newCappedDigestWriter(limit int64) *cappedDigestWriter {
	return &cappedDigestWriter{limit: limit, hash: sha256.New()}
}

func (w *cappedDigestWriter) Write(p []byte) (int, error) {
	_, _ = w.hash.Write(p)
	w.total += int64(len(p))
	remaining := w.limit - int64(w.buffer.Len())
	if remaining > 0 {
		toStore := p
		if int64(len(toStore)) > remaining {
			toStore = toStore[:remaining]
			w.truncated = true
		}
		_, _ = w.buffer.Write(toStore)
	}
	if int64(w.buffer.Len()) >= w.limit && w.total > int64(w.buffer.Len()) {
		w.truncated = true
	}
	return len(p), nil
}

func (w *cappedDigestWriter) Stored() []byte  { return append([]byte(nil), w.buffer.Bytes()...) }
func (w *cappedDigestWriter) Total() int64    { return w.total }
func (w *cappedDigestWriter) Truncated() bool { return w.truncated }
func (w *cappedDigestWriter) Digest() string {
	return "sha256:" + hex.EncodeToString(w.hash.Sum(nil))
}

// errorsAs is a tiny seam for tests and keeps the runner logic readable.
func errorsAs(err error, target any) bool {
	return errors.As(err, target)
}
