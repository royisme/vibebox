package vibebox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"vibebox/internal/backend"
	dockerbackend "vibebox/internal/backend/docker"
	macosbackend "vibebox/internal/backend/macos"
	offbackend "vibebox/internal/backend/off"
	"vibebox/internal/config"
	"vibebox/internal/image"
	"vibebox/internal/progress"
)

// Service is the public application-layer entrypoint for embedding vibebox.
type Service struct{}

// NewService creates a new application service.
func NewService() *Service {
	return &Service{}
}

// ListImages returns official white-listed images for the provided architecture.
// If hostArch is empty, runtime.GOARCH is used.
func (s *Service) ListImages(hostArch string) []Image {
	if hostArch == "" {
		hostArch = runtime.GOARCH
	}
	desc := image.ListForArch(hostArch)
	out := make([]Image, 0, len(desc))
	for _, d := range desc {
		out = append(out, toPublicImage(d))
	}
	return out
}

// ResolveDefaultImage returns the first official image for the given architecture.
func (s *Service) ResolveDefaultImage(hostArch string) (Image, error) {
	images := s.ListImages(hostArch)
	if len(images) == 0 {
		return Image{}, fmt.Errorf("no official images available for arch=%s", hostArch)
	}
	return images[0], nil
}

// Initialize prepares image artifacts and writes project config.
func (s *Service) Initialize(ctx context.Context, req InitializeRequest) (InitializeResult, error) {
	projectRoot, err := resolveProjectRoot(req.ProjectRoot)
	if err != nil {
		return InitializeResult{}, err
	}

	desc, err := resolveImageDescriptor(req.ImageID)
	if err != nil {
		return InitializeResult{}, err
	}
	if desc.Arch != runtime.GOARCH {
		return InitializeResult{}, fmt.Errorf("image %s is for arch=%s, host arch=%s", desc.ID, desc.Arch, runtime.GOARCH)
	}

	provider, err := normalizeProvider(req.Provider)
	if err != nil {
		return InitializeResult{}, err
	}

	manager, err := image.NewManager()
	if err != nil {
		return InitializeResult{}, err
	}

	sink := progress.FuncSink(func(e progress.Event) {
		emit(req.OnEvent, Event{
			Kind:       "init.progress",
			Phase:      string(e.Phase),
			Message:    e.Message,
			Percent:    e.Percent,
			BytesDone:  e.BytesDone,
			BytesTotal: e.BytesTotal,
			SpeedBps:   e.SpeedBps,
			ETA:        e.ETA,
			Err:        e.Err,
			Done:       e.Done,
		})
	})

	prepared, err := manager.EnsurePrepared(ctx, desc, sink)
	if err != nil {
		return InitializeResult{}, err
	}

	cfg := config.Default()
	cfg.Provider = toInternalProvider(provider)
	cfg.VM.ImageID = desc.ID
	cfg.VM.ImageVersion = desc.Version
	if req.CPUs > 0 {
		cfg.VM.CPUs = req.CPUs
	}
	if req.RAMMB > 0 {
		cfg.VM.RAMMB = req.RAMMB
	}
	if req.DiskGB > 0 {
		cfg.VM.DiskGB = req.DiskGB
	}
	if err := cfg.Validate(); err != nil {
		return InitializeResult{}, err
	}

	configPath := config.ProjectConfigPath(projectRoot)
	if err := config.Save(configPath, cfg); err != nil {
		return InitializeResult{}, err
	}

	result := InitializeResult{
		ProjectRoot: projectRoot,
		ConfigPath:  configPath,
		Image:       toPublicImage(desc),
		BaseRawPath: prepared.RawPath,
	}
	emit(req.OnEvent, Event{Kind: "init.completed", Message: "initialization completed", Done: true})
	return result, nil
}

// Probe evaluates backend availability and provider selection.
func (s *Service) Probe(ctx context.Context, provider Provider) (ProbeResult, error) {
	normalized, err := normalizeProvider(provider)
	if err != nil {
		return ProbeResult{}, err
	}

	off := offbackend.New()
	appleVM := macosbackend.New()
	docker := dockerbackend.New()
	selection, selErr := backend.Select(ctx, toInternalProvider(normalized), off, appleVM, docker)

	result := ProbeResult{
		Diagnostics: map[string]BackendDiagnostic{},
	}
	result.Diagnostics[off.Name()] = fromInternalDiag(off.Probe(ctx))
	result.Diagnostics[appleVM.Name()] = fromInternalDiag(appleVM.Probe(ctx))
	result.Diagnostics[docker.Name()] = fromInternalDiag(docker.Probe(ctx))

	if selErr != nil {
		return result, selErr
	}

	result.Selected = Provider(selection.Provider)
	result.WasFallback = selection.WasFallback
	result.FallbackFrom = selection.FallbackFrom
	return result, nil
}

// Start launches sandbox session using configured or overridden provider.
func (s *Service) Start(ctx context.Context, req StartRequest) (StartResult, error) {
	projectRoot, cfg, baseRaw, err := s.resolveProjectRuntime(req.ProjectRoot, req.ProviderOverride, true)
	if err != nil {
		return StartResult{}, err
	}

	off := offbackend.New()
	appleVM := macosbackend.New()
	docker := dockerbackend.New()

	provider := Provider(cfg.Provider)
	if req.ProviderOverride != "" {
		provider, err = normalizeProvider(req.ProviderOverride)
		if err != nil {
			return StartResult{}, err
		}
	}

	selection, err := backend.Select(ctx, toInternalProvider(provider), off, appleVM, docker)
	if err != nil {
		return StartResult{}, err
	}

	result := StartResult{
		Selected:     Provider(selection.Provider),
		WasFallback:  selection.WasFallback,
		FallbackFrom: selection.FallbackFrom,
		Diagnostics:  map[string]BackendDiagnostic{},
	}
	for name, diag := range selection.Diagnostics {
		result.Diagnostics[name] = fromInternalDiag(diag)
	}

	if selection.WasFallback {
		emit(req.OnEvent, Event{Kind: "start.fallback", Message: fmt.Sprintf("fallback from %s to %s", selection.FallbackFrom, selection.Backend.Name())})
	}

	spec := backend.RuntimeSpec{
		ProjectRoot: projectRoot,
		ProjectName: filepath.Base(projectRoot),
		Config:      cfg,
		BaseRawPath: baseRaw,
		InstanceRaw: config.InstanceDiskPath(projectRoot),
		IO: backend.IOStreams{
			Stdin:  req.IO.Stdin,
			Stdout: req.IO.Stdout,
			Stderr: req.IO.Stderr,
		},
	}

	emit(req.OnEvent, Event{Kind: "start.prepare", Message: "preparing backend"})
	if err := selection.Backend.Prepare(ctx, spec); err != nil {
		return StartResult{}, err
	}
	emit(req.OnEvent, Event{Kind: "start.running", Message: fmt.Sprintf("starting %s backend", selection.Backend.Name())})
	if err := selection.Backend.Start(ctx, spec); err != nil {
		return StartResult{}, err
	}
	emit(req.OnEvent, Event{Kind: "start.completed", Message: "sandbox session ended", Done: true})
	return result, nil
}

// Exec executes one command non-interactively and returns deterministic output.
func (s *Service) Exec(ctx context.Context, req ExecRequest) (ExecResult, error) {
	if req.Command == "" {
		return ExecResult{}, fmt.Errorf("command is required")
	}

	projectRoot, cfg, baseRaw, err := s.resolveProjectRuntime(req.ProjectRoot, req.ProviderOverride, false)
	if err != nil {
		return ExecResult{}, err
	}

	off := offbackend.New()
	appleVM := macosbackend.New()
	docker := dockerbackend.New()

	provider := Provider(cfg.Provider)
	if req.ProviderOverride != "" {
		provider, err = normalizeProvider(req.ProviderOverride)
		if err != nil {
			return ExecResult{}, err
		}
	}

	selection, err := backend.Select(ctx, toInternalProvider(provider), off, appleVM, docker)
	if err != nil {
		return ExecResult{}, err
	}

	diagnostics := map[string]BackendDiagnostic{}
	for name, diag := range selection.Diagnostics {
		diagnostics[name] = fromInternalDiag(diag)
	}

	spec := backend.RuntimeSpec{
		ProjectRoot: projectRoot,
		ProjectName: filepath.Base(projectRoot),
		Config:      cfg,
		BaseRawPath: baseRaw,
		InstanceRaw: config.InstanceDiskPath(projectRoot),
	}

	emit(req.OnEvent, Event{Kind: "exec.prepare", Message: "preparing backend"})
	if err := selection.Backend.Prepare(ctx, spec); err != nil {
		return ExecResult{}, err
	}

	execCtx := ctx
	timeout := time.Duration(0)
	if req.TimeoutSeconds > 0 {
		timeout = time.Duration(req.TimeoutSeconds) * time.Second
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	emit(req.OnEvent, Event{Kind: "exec.running", Message: fmt.Sprintf("executing via %s", selection.Backend.Name())})
	beResult, err := selection.Backend.Exec(execCtx, spec, backend.ExecRequest{
		Command: req.Command,
		Cwd:     req.Cwd,
		Env:     req.Env,
		Timeout: timeout,
	})
	if err != nil {
		return ExecResult{}, err
	}

	result := ExecResult{
		Stdout:      beResult.Stdout,
		Stderr:      beResult.Stderr,
		ExitCode:    beResult.ExitCode,
		Selected:    Provider(selection.Provider),
		Diagnostics: diagnostics,
	}
	emit(req.OnEvent, Event{Kind: "exec.completed", Message: "command execution completed", Done: true})
	return result, nil
}

func (s *Service) resolveProjectRuntime(projectRootInput string, providerOverride Provider, requireInitialized bool) (string, config.Config, string, error) {
	projectRoot, err := resolveProjectRoot(projectRootInput)
	if err != nil {
		return "", config.Config{}, "", err
	}

	cfgPath := config.ProjectConfigPath(projectRoot)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return "", config.Config{}, "", err
		}
		if requireInitialized {
			return "", config.Config{}, "", fmt.Errorf("project is not initialized. run `vibebox init`")
		}
		cfg = config.Default()
		if providerOverride == ProviderOff {
			cfg.Provider = config.ProviderOff
		}
	}

	lockPath, err := config.UserLockPath()
	if err != nil {
		return "", config.Config{}, "", err
	}
	lock, err := config.LoadImageLock(lockPath)
	if err != nil {
		return "", config.Config{}, "", err
	}

	baseRaw := ""
	if cfg.VM.ImageID != "" && cfg.VM.ImageVersion != "" {
		if ref, ok := lock.Images[config.LockKey(cfg.VM.ImageID, cfg.VM.ImageVersion)]; ok {
			baseRaw = ref.RawPath
		}
	}

	return projectRoot, cfg, baseRaw, nil
}

func resolveProjectRoot(root string) (string, error) {
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return cwd, nil
	}
	if filepath.IsAbs(root) {
		return root, nil
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func resolveImageDescriptor(imageID string) (image.Descriptor, error) {
	if imageID != "" {
		desc, ok := image.FindByID(imageID)
		if !ok {
			return image.Descriptor{}, fmt.Errorf("unknown image id: %s", imageID)
		}
		return desc, nil
	}
	images := image.ListForArch(runtime.GOARCH)
	if len(images) == 0 {
		return image.Descriptor{}, fmt.Errorf("no official images available for arch=%s", runtime.GOARCH)
	}
	return images[0], nil
}

func normalizeProvider(p Provider) (Provider, error) {
	if p == "" {
		return ProviderAuto, nil
	}
	switch p {
	case ProviderOff, ProviderAuto, ProviderAppleVM, ProviderDocker:
		return p, nil
	case ProviderMacOS:
		return ProviderAppleVM, nil
	default:
		return "", fmt.Errorf("invalid provider: %q", p)
	}
}

func toInternalProvider(p Provider) config.Provider {
	if p == "" {
		return config.ProviderAuto
	}
	return config.NormalizeProvider(config.Provider(p))
}

func fromInternalDiag(d backend.ProbeResult) BackendDiagnostic {
	return BackendDiagnostic{
		Available: d.Available,
		Reason:    d.Reason,
		FixHints:  d.FixHints,
	}
}

func toPublicImage(d image.Descriptor) Image {
	return Image{
		ID:          d.ID,
		DisplayName: d.DisplayName,
		Version:     d.Version,
		Arch:        d.Arch,
		URL:         d.URL,
		SizeBytes:   d.SizeBytes,
	}
}

func emit(handler EventHandler, e Event) {
	if handler != nil {
		handler(e)
	}
}
