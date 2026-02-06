package off

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"vibebox/internal/backend"
)

// Backend executes commands directly on host with conservative policy defaults.
type Backend struct{}

type sessionHandle struct {
	cwd string
	env map[string]string
}

func New() *Backend {
	return &Backend{}
}

func (b *Backend) Name() string {
	return "off"
}

func (b *Backend) Probe(ctx context.Context) backend.ProbeResult {
	_ = ctx
	if _, err := exec.LookPath("/bin/bash"); err != nil {
		return backend.ProbeResult{
			Available: false,
			Reason:    "/bin/bash not found",
			FixHints:  []string{"install bash or configure shell path"},
		}
	}
	return backend.ProbeResult{Available: true}
}

func (b *Backend) Prepare(ctx context.Context, spec backend.RuntimeSpec) error {
	_ = ctx
	_ = spec
	return nil
}

func (b *Backend) Start(ctx context.Context, spec backend.RuntimeSpec) error {
	cmd := exec.CommandContext(ctx, "/bin/bash")
	cmd.Dir = spec.ProjectRoot
	cmd.Stdin = spec.IO.Stdin
	cmd.Stdout = spec.IO.Stdout
	cmd.Stderr = spec.IO.Stderr
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

func (b *Backend) Exec(ctx context.Context, spec backend.RuntimeSpec, req backend.ExecRequest) (backend.ExecResult, error) {
	hostCwd, err := resolveHostCwd(spec.ProjectRoot, req.Cwd)
	if err != nil {
		return backend.ExecResult{}, err
	}

	cmd := exec.CommandContext(ctx, "/bin/bash", "-lc", req.Command)
	cmd.Dir = hostCwd
	cmd.Env = mergeRestrictedEnv(req.Env)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	result := backend.ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}
	if err == nil {
		return result, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	return result, err
}

func (b *Backend) StartSession(ctx context.Context, spec backend.RuntimeSpec, req backend.SessionStartRequest) (backend.SessionHandle, error) {
	_ = ctx
	hostCwd, err := resolveHostCwd(spec.ProjectRoot, req.Cwd)
	if err != nil {
		return nil, err
	}
	return sessionHandle{
		cwd: hostCwd,
		env: cloneMap(req.Env),
	}, nil
}

func (b *Backend) ExecInSession(ctx context.Context, spec backend.RuntimeSpec, handle backend.SessionHandle, req backend.ExecRequest) (backend.ExecResult, error) {
	h, ok := handle.(sessionHandle)
	if !ok {
		return backend.ExecResult{}, fmt.Errorf("invalid off session handle")
	}
	effectiveCwd := req.Cwd
	if effectiveCwd == "" {
		effectiveCwd = h.cwd
	}
	effectiveEnv := cloneMap(h.env)
	for k, v := range req.Env {
		effectiveEnv[k] = v
	}
	return b.Exec(ctx, spec, backend.ExecRequest{
		Command: req.Command,
		Cwd:     effectiveCwd,
		Env:     effectiveEnv,
		Timeout: req.Timeout,
	})
}

func (b *Backend) StopSession(ctx context.Context, spec backend.RuntimeSpec, handle backend.SessionHandle) error {
	_ = ctx
	_ = spec
	_ = handle
	return nil
}

func resolveHostCwd(projectRoot string, requested string) (string, error) {
	if requested == "" {
		return projectRoot, nil
	}

	var host string
	if filepath.IsAbs(requested) {
		host = filepath.Clean(requested)
	} else {
		host = filepath.Clean(filepath.Join(projectRoot, requested))
	}

	rel, err := filepath.Rel(projectRoot, host)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("cwd %s escapes project root %s", host, projectRoot)
	}
	info, err := os.Stat(host)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("cwd is not a directory: %s", host)
	}
	return host, nil
}

func mergeRestrictedEnv(extra map[string]string) []string {
	allow := map[string]bool{
		"PATH": true, "HOME": true, "USER": true, "SHELL": true,
		"LANG": true, "LC_ALL": true, "TMPDIR": true,
	}
	base := map[string]string{}
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k, v := parts[0], parts[1]
		if allow[k] {
			base[k] = v
		}
	}
	for k, v := range extra {
		base[k] = v
	}
	keys := make([]string, 0, len(base))
	for k := range base {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+base[k])
	}
	return out
}

func cloneMap(in map[string]string) map[string]string {
	if in == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
