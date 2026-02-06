package macos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"vibebox/internal/backend"
)

const (
	workspaceGuestPath = "/workspace"
	exitCodeMarker     = "__VIBEBOX_EXIT_CODE__"
)

// Backend implements the apple-vm provider.
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

func (b *Backend) Prepare(ctx context.Context, spec backend.RuntimeSpec) error {
	if _, err := os.Stat(spec.BaseRawPath); err != nil {
		return fmt.Errorf("base raw image missing: %w", err)
	}
	created := false
	if _, err := os.Stat(spec.InstanceRaw); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(spec.InstanceRaw), 0o755); err != nil {
		return err
	}
	if err := copyFile(spec.BaseRawPath, spec.InstanceRaw); err != nil {
		return fmt.Errorf("create instance disk: %w", err)
	}
	created = true
	if created {
		if err := b.provisionInstance(ctx, spec); err != nil {
			_ = os.Remove(spec.InstanceRaw)
			return fmt.Errorf("provision instance disk: %w", err)
		}
	}
	return nil
}

func (b *Backend) StartSession(ctx context.Context, spec backend.RuntimeSpec, req backend.SessionStartRequest) (backend.SessionHandle, error) {
	_ = ctx
	workspaceGuest := workspaceGuestFromSpec(spec)
	if req.Cwd != "" && !strings.HasPrefix(req.Cwd, "/") {
		projectGuest, ok := projectRootGuestFromSpec(spec)
		if !ok {
			return nil, fmt.Errorf("relative cwd requires a mount for project root %s", spec.ProjectRoot)
		}
		workspaceGuest = projectGuest
	}
	guestCwd, err := resolveVMGuestCwd(spec.ProjectRoot, req.Cwd, workspaceGuest)
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
	// Transitional mode: each exec runs in an isolated VM lifecycle.
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

func projectRootGuestFromSpec(spec backend.RuntimeSpec) (string, bool) {
	projectRootClean := filepath.Clean(spec.ProjectRoot)
	for _, m := range spec.Config.Mounts {
		if m.Guest == "" {
			continue
		}
		hostPath := m.Host
		if hostPath == "" {
			continue
		}
		if !filepath.IsAbs(hostPath) {
			hostPath = filepath.Join(spec.ProjectRoot, hostPath)
		}
		if filepath.Clean(hostPath) == projectRootClean {
			return m.Guest, true
		}
	}
	return "", false
}

func workspaceGuestFromSpec(spec backend.RuntimeSpec) string {
	if guest, ok := projectRootGuestFromSpec(spec); ok {
		return guest
	}
	for _, m := range spec.Config.Mounts {
		if m.Guest != "" {
			return m.Guest
		}
	}
	return workspaceGuestPath
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
