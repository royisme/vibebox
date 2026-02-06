package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"gopkg.in/yaml.v3"
)

// Provider controls which sandbox backend should be used.
type Provider string

const (
	ProviderOff     Provider = "off"
	ProviderAuto    Provider = "auto"
	ProviderAppleVM Provider = "apple-vm"
	ProviderMacOS   Provider = "macos" // legacy alias, normalized to apple-vm.
	ProviderDocker  Provider = "docker"
)

func (p Provider) Validate() error {
	switch p {
	case ProviderOff, ProviderAuto, ProviderAppleVM, ProviderDocker:
		return nil
	default:
		return fmt.Errorf("invalid provider: %q", p)
	}
}

// NormalizeProvider maps legacy provider names to canonical values.
func NormalizeProvider(p Provider) Provider {
	switch p {
	case ProviderMacOS:
		return ProviderAppleVM
	default:
		return p
	}
}

// Config is the project-level vibebox configuration.
type Config struct {
	Provider Provider     `yaml:"provider"`
	VM       VMConfig     `yaml:"vm"`
	Docker   DockerConfig `yaml:"docker"`
	Mounts   []Mount      `yaml:"mounts"`
}

// VMConfig stores VM backend settings.
type VMConfig struct {
	ImageID      string `yaml:"image_id"`
	ImageVersion string `yaml:"image_version"`
	DiskGB       int    `yaml:"disk_gb"`
	CPUs         int    `yaml:"cpus"`
	RAMMB        int    `yaml:"ram_mb"`
	// ProvisionScript is an optional host script path executed once when creating project instance disk.
	ProvisionScript string `yaml:"provision_script,omitempty"`
}

// DockerConfig stores Docker backend settings.
type DockerConfig struct {
	Image string `yaml:"image"`
}

// Mount represents a host-to-guest mount.
type Mount struct {
	Host  string `yaml:"host"`
	Guest string `yaml:"guest"`
	Mode  string `yaml:"mode"`
}

func Default() Config {
	defaultDockerImage := "debian:13"
	if runtime.GOARCH == "arm64" {
		defaultDockerImage = "arm64v8/debian:13"
	}

	return Config{
		Provider: ProviderAuto,
		VM: VMConfig{
			DiskGB: 20,
			CPUs:   2,
			RAMMB:  2048,
		},
		Docker: DockerConfig{
			Image: defaultDockerImage,
		},
		Mounts: []Mount{{
			Host:  ".",
			Guest: "/workspace",
			Mode:  "rw",
		}},
	}
}

func (c *Config) Validate() error {
	c.Provider = NormalizeProvider(c.Provider)
	if err := c.Provider.Validate(); err != nil {
		return err
	}
	if c.Provider == ProviderAuto || c.Provider == ProviderAppleVM {
		if c.VM.CPUs < 1 {
			return errors.New("vm.cpus must be >= 1")
		}
		if c.VM.RAMMB < 256 {
			return errors.New("vm.ram_mb must be >= 256")
		}
		if c.VM.DiskGB < 1 {
			return errors.New("vm.disk_gb must be >= 1")
		}
	}
	if c.Provider == ProviderAuto || c.Provider == ProviderDocker {
		if c.Docker.Image == "" {
			return errors.New("docker.image is required")
		}
	}
	for _, m := range c.Mounts {
		if m.Host == "" || m.Guest == "" {
			return errors.New("mount.host and mount.guest are required")
		}
		if m.Mode != "ro" && m.Mode != "rw" {
			return fmt.Errorf("invalid mount mode for %s: %s", m.Host, m.Mode)
		}
	}
	return nil
}

// ProjectConfigPath returns the path to the project-level config file.
func ProjectConfigPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".vibebox", "config.yaml")
}

// ProjectStateDir returns .vibebox for the current project.
func ProjectStateDir(projectRoot string) string {
	return filepath.Join(projectRoot, ".vibebox")
}

// InstanceDiskPath returns project instance disk path.
func InstanceDiskPath(projectRoot string) string {
	return filepath.Join(ProjectStateDir(projectRoot), "instance.raw")
}

// UserLockPath returns the image lock file location.
func UserLockPath() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfgDir, "vibebox", "images.lock.yaml"), nil
}

// UserCacheDir returns vibebox cache directory path.
func UserCacheDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "vibebox"), nil
}

func Load(path string) (Config, error) {
	var cfg Config
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}
	if cfg.Provider == "" {
		cfg.Provider = ProviderAuto
	}
	cfg.Provider = NormalizeProvider(cfg.Provider)
	if cfg.VM.CPUs == 0 || cfg.VM.RAMMB == 0 || cfg.VM.DiskGB == 0 {
		defaults := Default()
		if cfg.VM.CPUs == 0 {
			cfg.VM.CPUs = defaults.VM.CPUs
		}
		if cfg.VM.RAMMB == 0 {
			cfg.VM.RAMMB = defaults.VM.RAMMB
		}
		if cfg.VM.DiskGB == 0 {
			cfg.VM.DiskGB = defaults.VM.DiskGB
		}
	}
	if cfg.Docker.Image == "" {
		cfg.Docker.Image = Default().Docker.Image
	}
	if len(cfg.Mounts) == 0 {
		cfg.Mounts = Default().Mounts
	}
	return cfg, cfg.Validate()
}

func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

// ImageLock tracks downloaded and verified images.
type ImageLock struct {
	UpdatedAt time.Time               `yaml:"updated_at"`
	Images    map[string]ImageLockRef `yaml:"images"`
}

// ImageLockRef describes a cached image artifact.
type ImageLockRef struct {
	ID           string    `yaml:"id"`
	Version      string    `yaml:"version"`
	SHA256       string    `yaml:"sha256"`
	ArtifactPath string    `yaml:"artifact_path"`
	RawPath      string    `yaml:"raw_path"`
	DownloadedAt time.Time `yaml:"downloaded_at"`
}

func LockKey(imageID, version string) string {
	return imageID + "@" + version
}

func LoadImageLock(path string) (ImageLock, error) {
	lock := ImageLock{Images: map[string]ImageLockRef{}}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return lock, nil
		}
		return lock, err
	}
	if err := yaml.Unmarshal(raw, &lock); err != nil {
		return lock, err
	}
	if lock.Images == nil {
		lock.Images = map[string]ImageLockRef{}
	}
	return lock, nil
}

func SaveImageLock(path string, lock ImageLock) error {
	if lock.Images == nil {
		lock.Images = map[string]ImageLockRef{}
	}
	lock.UpdatedAt = time.Now().UTC()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := yaml.Marshal(&lock)
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}
