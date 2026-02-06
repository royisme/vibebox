package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := Default()
	cfg.Provider = ProviderDocker
	cfg.VM.ImageID = "debian-13-nocloud-arm64"
	cfg.VM.ImageVersion = "20260112-2355"

	if err := Save(path, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.Provider != ProviderDocker {
		t.Fatalf("provider mismatch: got %s", loaded.Provider)
	}
	if loaded.VM.ImageID != cfg.VM.ImageID {
		t.Fatalf("image id mismatch: got %s", loaded.VM.ImageID)
	}
}

func TestLoadLegacyMacOSProviderAlias(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := []byte("provider: macos\nvm:\n  disk_gb: 20\n  cpus: 2\n  ram_mb: 2048\ndocker:\n  image: debian:13\nmounts:\n  - host: .\n    guest: /workspace\n    mode: rw\n")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Provider != ProviderAppleVM {
		t.Fatalf("expected provider apple-vm, got %s", cfg.Provider)
	}
}
