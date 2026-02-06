package image

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadAndVerify(t *testing.T) {
	t.Parallel()
	payload := []byte("vibebox-test-payload")
	h := sha256.Sum256(payload)
	sum := hex.EncodeToString(h[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "20")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "artifact.bin")

	err := DownloadAndVerify(context.Background(), DownloadRequest{
		URL:            srv.URL,
		DestPath:       dest,
		ExpectedSHA256: sum,
		ExpectedBytes:  int64(len(payload)),
	})
	if err != nil {
		t.Fatalf("download: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload mismatch")
	}
}
