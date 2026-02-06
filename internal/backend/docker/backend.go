package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"vibebox/internal/backend"
)

// Backend implements Docker runtime.
type Backend struct{}

func New() *Backend {
	return &Backend{}
}

func (b *Backend) Name() string {
	return "docker"
}

func (b *Backend) Probe(ctx context.Context) backend.ProbeResult {
	if _, err := exec.LookPath("docker"); err != nil {
		return backend.ProbeResult{
			Available: false,
			Reason:    "docker command not found",
			FixHints:  []string{"install Docker Desktop or docker engine", "ensure docker is on PATH"},
		}
	}

	cmd := exec.CommandContext(ctx, "docker", "info")
	if err := cmd.Run(); err != nil {
		return backend.ProbeResult{
			Available: false,
			Reason:    "docker daemon not reachable",
			FixHints:  []string{"start docker daemon", "run `docker info` and fix errors"},
		}
	}

	return backend.ProbeResult{Available: true}
}

func (b *Backend) Prepare(ctx context.Context, spec backend.RuntimeSpec) error {
	inspect := exec.CommandContext(ctx, "docker", "image", "inspect", spec.Config.Docker.Image)
	if err := inspect.Run(); err == nil {
		return nil
	}
	pull := exec.CommandContext(ctx, "docker", "pull", spec.Config.Docker.Image)
	pullStdout := spec.IO.Stdout
	pullStderr := spec.IO.Stderr
	if pullStdout == nil {
		pullStdout = os.Stderr
	}
	if pullStderr == nil {
		pullStderr = os.Stderr
	}
	pull.Stdout = pullStdout
	pull.Stderr = pullStderr
	if err := pull.Run(); err != nil {
		return fmt.Errorf("pull docker image %s: %w", spec.Config.Docker.Image, err)
	}
	return nil
}

func (b *Backend) Start(ctx context.Context, spec backend.RuntimeSpec) error {
	containerName := "vibebox-" + sanitizeName(spec.ProjectName)

	args := []string{"run", "--rm", "-it", "--name", containerName, "-e", "IS_SANDBOX=1"}
	for _, m := range spec.Config.Mounts {
		hostPath := m.Host
		if !filepath.IsAbs(hostPath) {
			hostPath = filepath.Join(spec.ProjectRoot, hostPath)
		}
		if _, err := os.Stat(hostPath); err != nil {
			return fmt.Errorf("mount host path does not exist: %s", hostPath)
		}
		args = append(args, "-v", fmt.Sprintf("%s:%s:%s", hostPath, m.Guest, m.Mode))
	}

	args = append(args,
		"-w", "/workspace",
		spec.Config.Docker.Image,
		"/bin/bash",
	)

	cmd := exec.CommandContext(ctx, "docker", args...)
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
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("docker exited with code %d", exitErr.ExitCode())
		}
		return err
	}
	return nil
}

func (b *Backend) Exec(ctx context.Context, spec backend.RuntimeSpec, req backend.ExecRequest) (backend.ExecResult, error) {
	workspaceGuest := "/workspace"
	guestCwd, err := resolveGuestCwd(spec.ProjectRoot, req.Cwd, workspaceGuest)
	if err != nil {
		return backend.ExecResult{}, err
	}

	args := []string{"run", "--rm", "-i", "-e", "IS_SANDBOX=1"}
	for _, m := range spec.Config.Mounts {
		hostPath := m.Host
		if !filepath.IsAbs(hostPath) {
			hostPath = filepath.Join(spec.ProjectRoot, hostPath)
		}
		if _, err := os.Stat(hostPath); err != nil {
			return backend.ExecResult{}, fmt.Errorf("mount host path does not exist: %s", hostPath)
		}
		args = append(args, "-v", fmt.Sprintf("%s:%s:%s", hostPath, m.Guest, m.Mode))
	}
	for _, e := range envList(req.Env) {
		args = append(args, "-e", e)
	}
	args = append(args,
		"-w", guestCwd,
		spec.Config.Docker.Image,
		"/bin/bash", "-lc", req.Command,
	)

	cmd := exec.CommandContext(ctx, "docker", args...)
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
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	return result, err
}

func resolveGuestCwd(projectRoot, requested, workspaceGuest string) (string, error) {
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

func envList(extra map[string]string) []string {
	if len(extra) == 0 {
		return nil
	}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+extra[k])
	}
	return out
}

func sanitizeName(in string) string {
	if in == "" {
		return "project"
	}
	in = strings.ToLower(in)
	in = strings.ReplaceAll(in, " ", "-")
	builder := strings.Builder{}
	for _, ch := range in {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			builder.WriteRune(ch)
		}
	}
	out := builder.String()
	if out == "" {
		return "project"
	}
	return out
}
