package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/term"

	"vibebox/internal/backend"
	dockerbackend "vibebox/internal/backend/docker"
	macosbackend "vibebox/internal/backend/macos"
	offbackend "vibebox/internal/backend/off"
	"vibebox/internal/config"
	"vibebox/internal/image"
	"vibebox/internal/progress"
	"vibebox/internal/ui/tui"
)

// App coordinates command behavior.
type App struct {
	Stdout io.Writer
	Stderr io.Writer
}

func New(stdout, stderr io.Writer) *App {
	return &App{Stdout: stdout, Stderr: stderr}
}

// InitOptions controls `vibebox init` behavior.
type InitOptions struct {
	NonInteractive bool
	ImageID        string
	Provider       config.Provider
	CPUs           int
	RAMMB          int
	DiskGB         int
}

// UpOptions controls `vibebox up` behavior.
type UpOptions struct {
	Provider config.Provider
}

// UpgradeOptions controls `vibebox images upgrade` behavior.
type UpgradeOptions struct {
	ImageID string
}

func (a *App) Init(ctx context.Context, opts InitOptions) error {
	projectRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	images := image.ListForArch(runtime.GOARCH)
	if len(images) == 0 {
		return fmt.Errorf("no official images available for arch=%s", runtime.GOARCH)
	}

	chosen, err := pickImage(images, opts.ImageID, opts.NonInteractive)
	if err != nil {
		return err
	}

	manager, err := image.NewManager()
	if err != nil {
		return err
	}

	prepared, err := a.prepareImage(ctx, manager, chosen, opts.NonInteractive)
	if err != nil {
		return err
	}

	cfg := config.Default()
	cfg.Provider = opts.Provider
	cfg.VM.ImageID = chosen.ID
	cfg.VM.ImageVersion = chosen.Version
	if opts.CPUs > 0 {
		cfg.VM.CPUs = opts.CPUs
	}
	if opts.RAMMB > 0 {
		cfg.VM.RAMMB = opts.RAMMB
	}
	if opts.DiskGB > 0 {
		cfg.VM.DiskGB = opts.DiskGB
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	configPath := config.ProjectConfigPath(projectRoot)
	if err := config.Save(configPath, cfg); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(a.Stdout, "Initialized vibebox.\n")
	_, _ = fmt.Fprintf(a.Stdout, "  project config: %s\n", configPath)
	_, _ = fmt.Fprintf(a.Stdout, "  image: %s@%s\n", chosen.ID, chosen.Version)
	_, _ = fmt.Fprintf(a.Stdout, "  base raw: %s\n", prepared.RawPath)
	_, _ = fmt.Fprintln(a.Stdout, "Next: run `vibebox up`")
	return nil
}

func (a *App) Up(ctx context.Context, opts UpOptions) error {
	projectRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	projectName := filepath.Base(projectRoot)
	cfgPath := config.ProjectConfigPath(projectRoot)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("project is not initialized. run `vibebox init`")
		}
		return err
	}

	provider := cfg.Provider
	if opts.Provider != "" {
		provider = opts.Provider
	}

	lockPath, err := config.UserLockPath()
	if err != nil {
		return err
	}
	lock, err := config.LoadImageLock(lockPath)
	if err != nil {
		return err
	}

	baseRaw := ""
	if cfg.VM.ImageID != "" && cfg.VM.ImageVersion != "" {
		if ref, ok := lock.Images[config.LockKey(cfg.VM.ImageID, cfg.VM.ImageVersion)]; ok {
			baseRaw = ref.RawPath
		}
	}

	off := offbackend.New()
	mac := macosbackend.New()
	docker := dockerbackend.New()

	selection, err := backend.Select(ctx, provider, off, mac, docker)
	if err != nil {
		return err
	}

	if selection.WasFallback {
		_, _ = fmt.Fprintf(a.Stderr, "auto fallback: apple-vm backend unavailable, using docker\n")
	}

	spec := backend.RuntimeSpec{
		ProjectRoot: projectRoot,
		ProjectName: projectName,
		Config:      cfg,
		BaseRawPath: baseRaw,
		InstanceRaw: config.InstanceDiskPath(projectRoot),
		IO: backend.IOStreams{
			Stdin:  os.Stdin,
			Stdout: a.Stdout,
			Stderr: a.Stderr,
		},
	}

	if err := selection.Backend.Prepare(ctx, spec); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(a.Stdout, "Starting sandbox using %s backend...\n", selection.Backend.Name())
	return selection.Backend.Start(ctx, spec)
}

func (a *App) ImagesList() error {
	images := image.List()
	_, _ = fmt.Fprintln(a.Stdout, "ID\tARCH\tVERSION\tSIZE_MB")
	for _, d := range images {
		_, _ = fmt.Fprintf(a.Stdout, "%s\t%s\t%s\t%.1f\n", d.ID, d.Arch, d.Version, float64(d.SizeBytes)/1024.0/1024.0)
	}
	return nil
}

func (a *App) ImagesUpgrade(ctx context.Context, opts UpgradeOptions) error {
	var target image.Descriptor
	var ok bool
	if opts.ImageID != "" {
		target, ok = image.FindByID(opts.ImageID)
		if !ok {
			return fmt.Errorf("unknown image id: %s", opts.ImageID)
		}
	} else {
		projectRoot, err := os.Getwd()
		if err != nil {
			return err
		}
		cfg, err := config.Load(config.ProjectConfigPath(projectRoot))
		if err != nil {
			return fmt.Errorf("cannot infer image id: %w", err)
		}
		target, ok = image.FindByID(cfg.VM.ImageID)
		if !ok {
			return fmt.Errorf("current image id is not in official catalog: %s", cfg.VM.ImageID)
		}
	}

	manager, err := image.NewManager()
	if err != nil {
		return err
	}
	_, err = a.prepareImage(ctx, manager, target, false)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(a.Stdout, "image %s@%s is ready\n", target.ID, target.Version)
	return nil
}

func (a *App) prepareImage(ctx context.Context, manager *image.Manager, desc image.Descriptor, nonInteractive bool) (image.PreparedPaths, error) {
	interactive := !nonInteractive && isTerminal()
	events := make(chan progress.Event, 64)
	resultCh := make(chan struct {
		paths image.PreparedPaths
		err   error
	}, 1)

	sink := progress.FuncSink(func(e progress.Event) {
		events <- e
	})

	go func() {
		paths, err := manager.EnsurePrepared(ctx, desc, sink)
		if err != nil {
			events <- progress.Event{Phase: progress.PhaseFailed, Message: err.Error(), Err: err, Done: true}
		}
		close(events)
		resultCh <- struct {
			paths image.PreparedPaths
			err   error
		}{paths: paths, err: err}
	}()

	if interactive {
		if err := tui.RunProgress(events); err != nil {
			return image.PreparedPaths{}, err
		}
	} else {
		for e := range events {
			_, _ = fmt.Fprintln(a.Stdout, renderProgressLine(e))
		}
	}

	result := <-resultCh
	return result.paths, result.err
}

func renderProgressLine(e progress.Event) string {
	parts := []string{fmt.Sprintf("[%s]", e.Phase)}
	if e.Message != "" {
		parts = append(parts, e.Message)
	}
	if e.BytesTotal > 0 {
		parts = append(parts, fmt.Sprintf("%0.1f%%", e.Percent))
	}
	return strings.Join(parts, " ")
}

func pickImage(images []image.Descriptor, imageID string, nonInteractive bool) (image.Descriptor, error) {
	if imageID != "" {
		desc, ok := image.FindByID(imageID)
		if !ok {
			return image.Descriptor{}, fmt.Errorf("unknown image id: %s", imageID)
		}
		if desc.Arch != runtime.GOARCH {
			return image.Descriptor{}, fmt.Errorf("image %s is for arch=%s, host arch=%s", imageID, desc.Arch, runtime.GOARCH)
		}
		return desc, nil
	}

	if nonInteractive || !isTerminal() {
		return images[0], nil
	}

	return tui.SelectImage(images)
}

func isTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}
