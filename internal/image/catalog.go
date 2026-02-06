package image

import "runtime"

// BackendType indicates which backend can use this artifact.
type BackendType string

const (
	BackendVM     BackendType = "vm"
	BackendDocker BackendType = "docker"
)

// Descriptor defines one official white-listed VM image.
type Descriptor struct {
	ID           string
	DisplayName  string
	Version      string
	Arch         string
	URL          string
	ArtifactName string
	RawMember    string
	SHA256       string
	SizeBytes    int64
	Backend      BackendType
}

var catalog = []Descriptor{
	{
		ID:           "debian-13-nocloud-arm64",
		DisplayName:  "Debian 13 NoCloud (arm64)",
		Version:      "20260112-2355",
		Arch:         "arm64",
		URL:          "https://cloud.debian.org/images/cloud/trixie/20260112-2355/debian-13-nocloud-arm64-20260112-2355.tar.xz",
		ArtifactName: "debian-13-nocloud-arm64-20260112-2355.tar.xz",
		RawMember:    "disk.raw",
		SHA256:       "78924c6035bd54d3c2b0048b8397bba26286979a4ba9e8c7ab74663fa0e9584e",
		SizeBytes:    280901576,
		Backend:      BackendVM,
	},
	{
		ID:           "debian-13-nocloud-amd64",
		DisplayName:  "Debian 13 NoCloud (amd64)",
		Version:      "20260112-2355",
		Arch:         "amd64",
		URL:          "https://cloud.debian.org/images/cloud/trixie/20260112-2355/debian-13-nocloud-amd64-20260112-2355.tar.xz",
		ArtifactName: "debian-13-nocloud-amd64-20260112-2355.tar.xz",
		RawMember:    "disk.raw",
		SHA256:       "d19b6f4b4b6662c992d70cdda2ab98fde41a9f59d6531384cf1748075ee4571b",
		SizeBytes:    300592428,
		Backend:      BackendVM,
	},
}

// List returns all official catalog entries.
func List() []Descriptor {
	out := make([]Descriptor, len(catalog))
	copy(out, catalog)
	return out
}

// ListForArch returns entries compatible with the current or requested architecture.
func ListForArch(arch string) []Descriptor {
	if arch == "" {
		arch = runtime.GOARCH
	}
	var out []Descriptor
	for _, d := range catalog {
		if d.Arch == arch {
			out = append(out, d)
		}
	}
	return out
}

// FindByID returns an image descriptor by ID.
func FindByID(id string) (Descriptor, bool) {
	for _, d := range catalog {
		if d.ID == id {
			return d, true
		}
	}
	return Descriptor{}, false
}

// LatestByPrefix returns latest descriptor for a logical image family.
func LatestByPrefix(prefix string, arch string) (Descriptor, bool) {
	for _, d := range catalog {
		if d.Arch == arch && (d.ID == prefix || len(prefix) == 0) {
			return d, true
		}
	}
	return Descriptor{}, false
}
