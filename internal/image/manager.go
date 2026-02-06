package image

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"vibebox/internal/config"
	"vibebox/internal/progress"
)

// PreparedPaths stores artifact/raw cache paths.
type PreparedPaths struct {
	ArtifactPath string
	RawPath      string
}

// Manager manages image catalog downloads and cache state.
type Manager struct {
	CacheRoot string
	LockPath  string
}

func NewManager() (*Manager, error) {
	cacheRoot, err := config.UserCacheDir()
	if err != nil {
		return nil, err
	}
	lockPath, err := config.UserLockPath()
	if err != nil {
		return nil, err
	}
	return &Manager{CacheRoot: cacheRoot, LockPath: lockPath}, nil
}

// EnsurePrepared ensures artifact and extracted raw are present and verified.
func (m *Manager) EnsurePrepared(ctx context.Context, desc Descriptor, sink progress.Sink) (PreparedPaths, error) {
	if sink == nil {
		sink = progress.NopSink{}
	}

	imageDir := filepath.Join(m.CacheRoot, "images", desc.ID, desc.Version)
	artifact := filepath.Join(imageDir, desc.ArtifactName)
	rawPath := filepath.Join(imageDir, "base.raw")

	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		return PreparedPaths{}, err
	}

	if err := DownloadAndVerify(ctx, DownloadRequest{
		URL:            desc.URL,
		DestPath:       artifact,
		ExpectedSHA256: desc.SHA256,
		ExpectedBytes:  desc.SizeBytes,
		Sink:           sink,
	}); err != nil {
		return PreparedPaths{}, err
	}

	if _, err := os.Stat(rawPath); errors.Is(err, os.ErrNotExist) {
		sink.Emit(progress.Event{Phase: progress.PhasePreparing, Message: "extracting raw disk"})
		if err := extractTarMember(ctx, artifact, desc.RawMember, rawPath); err != nil {
			return PreparedPaths{}, err
		}
		sink.Emit(progress.Event{Phase: progress.PhasePreparing, Message: "raw disk ready", Percent: 100})
	}

	if err := m.updateLock(desc, artifact, rawPath); err != nil {
		return PreparedPaths{}, err
	}

	sink.Emit(progress.Event{Phase: progress.PhaseCompleted, Message: "image ready", Percent: 100, Done: true})
	return PreparedPaths{ArtifactPath: artifact, RawPath: rawPath}, nil
}

func extractTarMember(ctx context.Context, archivePath, member, outPath string) error {
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()

	cmd := exec.CommandContext(ctx, "tar", "-xOf", archivePath, member)
	cmd.Stdout = out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(outPath)
		return fmt.Errorf("extract %s from %s: %w", member, archivePath, err)
	}
	return nil
}

func (m *Manager) updateLock(desc Descriptor, artifactPath, rawPath string) error {
	lock, err := config.LoadImageLock(m.LockPath)
	if err != nil {
		return err
	}
	if lock.Images == nil {
		lock.Images = map[string]config.ImageLockRef{}
	}
	lock.Images[config.LockKey(desc.ID, desc.Version)] = config.ImageLockRef{
		ID:           desc.ID,
		Version:      desc.Version,
		SHA256:       desc.SHA256,
		ArtifactPath: artifactPath,
		RawPath:      rawPath,
		DownloadedAt: time.Now().UTC(),
	}
	return config.SaveImageLock(m.LockPath, lock)
}
