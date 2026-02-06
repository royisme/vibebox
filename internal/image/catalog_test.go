package image

import "testing"

func TestListForArch(t *testing.T) {
	t.Parallel()
	arm := ListForArch("arm64")
	if len(arm) == 0 {
		t.Fatalf("expected arm64 images")
	}
	for _, d := range arm {
		if d.Arch != "arm64" {
			t.Fatalf("unexpected arch: %s", d.Arch)
		}
	}
}
