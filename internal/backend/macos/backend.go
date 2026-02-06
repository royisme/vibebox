package macos

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"vibebox/internal/backend"
)

// Backend implements macOS VM runtime via a delegated vibe command.
type Backend struct{}

type sessionHandle struct {
	defaultCwd string
	defaultEnv map[string]string
}

func New() *Backend {
	return &Backend{}
}

func (b *Backend) Name() string {
	return "apple-vm"
}

func (b *Backend) Probe(ctx context.Context) backend.ProbeResult {
	if runtime.GOOS != "darwin" {
		return backend.ProbeResult{
			Available: false,
			Reason:    "apple-vm backend is only available on darwin",
			FixHints:  []string{"use provider=docker or provider=off on non-darwin hosts"},
		}
	}
	if _, err := exec.LookPath("vibe"); err != nil {
		return backend.ProbeResult{
			Available: false,
			Reason:    "vibe binary not found",
			FixHints: []string{
				"install vibe from https://github.com/lynaghk/vibe",
				"or use provider=docker",
			},
		}
	}

	// A lightweight check to ensure Virtualization entitlements are likely usable.
	if _, err := exec.LookPath("codesign"); err != nil {
		return backend.ProbeResult{
			Available: false,
			Reason:    "codesign command not found",
			FixHints:  []string{"install Xcode command line tools"},
		}
	}

	_ = ctx
	return backend.ProbeResult{Available: true}
}

func (b *Backend) Prepare(ctx context.Context, spec backend.RuntimeSpec) error {
	if _, err := os.Stat(spec.BaseRawPath); err != nil {
		return fmt.Errorf("base raw image missing: %w", err)
	}
	if _, err := os.Stat(spec.InstanceRaw); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(spec.InstanceRaw), 0o755); err != nil {
		return err
	}
	if err := copyFile(spec.BaseRawPath, spec.InstanceRaw); err != nil {
		return fmt.Errorf("create instance disk: %w", err)
	}
	_ = ctx
	return nil
}

func (b *Backend) Start(ctx context.Context, spec backend.RuntimeSpec) error {
	projectGuest := "/workspace"

	args := []string{
		spec.InstanceRaw,
		"--no-default-mounts",
		"--mount", fmt.Sprintf("%s:%s:read-write", spec.ProjectRoot, projectGuest),
		"--send", "cd /workspace",
		"--cpus", fmt.Sprintf("%d", spec.Config.VM.CPUs),
		"--ram", fmt.Sprintf("%d", spec.Config.VM.RAMMB),
	}

	cmd := exec.CommandContext(ctx, "vibe", args...)
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
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run vibe backend: %w", err)
	}
	return nil
}

func (b *Backend) Exec(ctx context.Context, spec backend.RuntimeSpec, req backend.ExecRequest) (backend.ExecResult, error) {
	projectGuest := "/workspace"
	guestCwd, err := resolveVMGuestCwd(spec.ProjectRoot, req.Cwd, projectGuest)
	if err != nil {
		return backend.ExecResult{}, err
	}

	const marker = "__VIBEBOX_EXIT_CODE__"
	envExports := shellExports(req.Env)
	command := fmt.Sprintf("cd %s && %s { %s ; }; rc=$?; echo %s$rc; poweroff", shellQuote(guestCwd), envExports, req.Command, marker)
	args := []string{
		spec.InstanceRaw,
		"--no-default-mounts",
		"--mount", fmt.Sprintf("%s:%s:read-write", spec.ProjectRoot, projectGuest),
		"--send", command,
		"--cpus", fmt.Sprintf("%d", spec.Config.VM.CPUs),
		"--ram", fmt.Sprintf("%d", spec.Config.VM.RAMMB),
	}

	cmd := exec.CommandContext(ctx, "vibe", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	result := backend.ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}
	if exitCode, ok := parseExitMarker(result.Stdout, marker); ok {
		result.ExitCode = exitCode
		result.Stdout = stripExitMarker(result.Stdout, marker)
		return result, nil
	}

	if runErr == nil {
		return result, nil
	}
	return result, fmt.Errorf("vibe exec failed: %w", runErr)
}

func (b *Backend) StartSession(ctx context.Context, spec backend.RuntimeSpec, req backend.SessionStartRequest) (backend.SessionHandle, error) {
	_ = ctx
	projectGuest := "/workspace"
	guestCwd, err := resolveVMGuestCwd(spec.ProjectRoot, req.Cwd, projectGuest)
	if err != nil {
		return nil, err
	}
	return sessionHandle{
		defaultCwd: guestCwd,
		defaultEnv: cloneMap(req.Env),
	}, nil
}

func (b *Backend) ExecInSession(ctx context.Context, spec backend.RuntimeSpec, handle backend.SessionHandle, req backend.ExecRequest) (backend.ExecResult, error) {
	h, ok := handle.(sessionHandle)
	if !ok {
		return backend.ExecResult{}, fmt.Errorf("invalid apple-vm session handle")
	}
	effectiveCwd := req.Cwd
	if effectiveCwd == "" {
		effectiveCwd = h.defaultCwd
	}
	env := cloneMap(h.defaultEnv)
	for k, v := range req.Env {
		env[k] = v
	}
	return b.Exec(ctx, spec, backend.ExecRequest{
		Command: req.Command,
		Cwd:     effectiveCwd,
		Env:     env,
		Timeout: req.Timeout,
	})
}

func (b *Backend) StopSession(ctx context.Context, spec backend.RuntimeSpec, handle backend.SessionHandle) error {
	_ = ctx
	_ = spec
	_ = handle
	// Transitional mode: delegated vibe backend does not keep a reusable VM session yet.
	return nil
}

func resolveVMGuestCwd(projectRoot, requested, workspaceGuest string) (string, error) {
	if requested == "" {
		return workspaceGuest, nil
	}
	if strings.HasPrefix(requested, "/") {
		return requested, nil
	}

	hostPath := filepath.Clean(filepath.Join(projectRoot, requested))
	rel, err := filepath.Rel(projectRoot, hostPath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("cwd %s escapes project root %s", hostPath, projectRoot)
	}
	return filepath.ToSlash(filepath.Join(workspaceGuest, rel)), nil
}

func parseExitMarker(output, marker string) (int, bool) {
	re := regexp.MustCompile(regexp.QuoteMeta(marker) + `(\d+)`)
	matches := re.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return 0, false
	}
	last := matches[len(matches)-1]
	code, err := strconv.Atoi(last[1])
	if err != nil {
		return 0, false
	}
	return code, true
}

func stripExitMarker(output, marker string) string {
	re := regexp.MustCompile(`(?m)^.*` + regexp.QuoteMeta(marker) + `\d+.*\n?`)
	return re.ReplaceAllString(output, "")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func shellExports(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, "export "+k+"="+shellQuote(env[k])+";")
	}
	return strings.Join(parts, " ") + " "
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

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = in.Close()
	}()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()

	if _, err := out.ReadFrom(in); err != nil {
		return err
	}
	return out.Sync()
}
