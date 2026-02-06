package image

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vibebox/internal/progress"
)

// DownloadRequest contains parameters for downloading and verifying one artifact.
type DownloadRequest struct {
	URL            string
	DestPath       string
	ExpectedSHA256 string
	ExpectedBytes  int64
	Sink           progress.Sink
}

// DownloadAndVerify downloads the file with resume support and validates SHA256.
func DownloadAndVerify(ctx context.Context, req DownloadRequest) error {
	sink := req.Sink
	if sink == nil {
		sink = progress.NopSink{}
	}

	if err := os.MkdirAll(filepath.Dir(req.DestPath), 0o755); err != nil {
		return err
	}

	existing := int64(0)
	if st, err := os.Stat(req.DestPath); err == nil {
		existing = st.Size()
	}

	sink.Emit(progress.Event{Phase: progress.PhaseResolving, Message: "resolving image source"})

	hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, req.URL, nil)
	if err != nil {
		return err
	}
	if existing > 0 {
		hreq.Header.Set("Range", fmt.Sprintf("bytes=%d-", existing))
	}

	resp, err := http.DefaultClient.Do(hreq)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	flags := os.O_CREATE | os.O_WRONLY
	if resp.StatusCode == http.StatusPartialContent && existing > 0 {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
		existing = 0
	}

	file, err := os.OpenFile(req.DestPath, flags, 0o644)
	if err != nil {
		return err
	}

	defer func() {
		_ = file.Close()
	}()

	total := req.ExpectedBytes
	if total <= 0 {
		if resp.ContentLength > 0 {
			total = existing + resp.ContentLength
		}
	}

	sink.Emit(progress.Event{
		Phase:      progress.PhaseDownloading,
		Message:    "downloading image",
		Percent:    percent(existing, total),
		BytesDone:  existing,
		BytesTotal: total,
	})

	writer := &progressWriter{sink: sink, total: total, done: existing, lastDone: existing, lastTick: time.Now()}
	if _, err := io.Copy(io.MultiWriter(file, writer), resp.Body); err != nil {
		return err
	}

	sink.Emit(progress.Event{
		Phase:      progress.PhaseDownloading,
		Message:    "download completed",
		Percent:    100,
		BytesDone:  writer.done,
		BytesTotal: total,
	})

	sink.Emit(progress.Event{Phase: progress.PhaseVerifying, Message: "verifying image digest"})
	actual, err := computeSHA256(req.DestPath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(actual, req.ExpectedSHA256) {
		_ = os.Remove(req.DestPath)
		return fmt.Errorf("sha256 mismatch: expected %s, got %s", req.ExpectedSHA256, actual)
	}

	sink.Emit(progress.Event{
		Phase:   progress.PhaseVerifying,
		Message: "digest verified",
		Percent: 100,
	})
	return nil
}

func computeSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = f.Close()
	}()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type progressWriter struct {
	sink     progress.Sink
	total    int64
	done     int64
	lastDone int64
	lastTick time.Time
}

func (w *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.done += int64(n)
	now := time.Now()
	if now.Sub(w.lastTick) >= 200*time.Millisecond {
		deltaBytes := w.done - w.lastDone
		deltaSeconds := now.Sub(w.lastTick).Seconds()
		speed := 0.0
		if deltaSeconds > 0 {
			speed = float64(deltaBytes) / deltaSeconds
		}
		eta := time.Duration(0)
		if speed > 0 && w.total > 0 {
			remaining := float64(w.total - w.done)
			if remaining > 0 {
				eta = time.Duration(remaining/speed) * time.Second
			}
		}

		w.sink.Emit(progress.Event{
			Phase:      progress.PhaseDownloading,
			Message:    "downloading image",
			Percent:    percent(w.done, w.total),
			BytesDone:  w.done,
			BytesTotal: w.total,
			SpeedBps:   speed,
			ETA:        eta,
		})
		w.lastTick = now
		w.lastDone = w.done
	}
	return n, nil
}

func percent(done, total int64) float64 {
	if total <= 0 {
		return 0
	}
	p := float64(done) * 100 / float64(total)
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}
