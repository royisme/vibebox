package vibebox

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNormalizeProvider(t *testing.T) {
	t.Parallel()
	if _, err := normalizeProvider("bad"); err == nil {
		t.Fatalf("expected error for invalid provider")
	}
	p, err := normalizeProvider("")
	if err != nil {
		t.Fatalf("normalize default: %v", err)
	}
	if p != ProviderAuto {
		t.Fatalf("expected auto, got %s", p)
	}
	alias, err := normalizeProvider(ProviderMacOS)
	if err != nil {
		t.Fatalf("normalize alias: %v", err)
	}
	if alias != ProviderAppleVM {
		t.Fatalf("expected alias to apple-vm, got %s", alias)
	}
}

func TestResolveDefaultImage(t *testing.T) {
	t.Parallel()
	svc := NewService()
	img, err := svc.ResolveDefaultImage(runtime.GOARCH)
	if err != nil {
		t.Fatalf("resolve image: %v", err)
	}
	if img.ID == "" {
		t.Fatalf("expected image id")
	}
	if img.Arch != runtime.GOARCH {
		t.Fatalf("unexpected arch: %s", img.Arch)
	}
}

func TestExecOffWithoutInit(t *testing.T) {
	t.Parallel()
	svc := NewService()
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "hello.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	result, err := svc.Exec(context.Background(), ExecRequest{
		ProjectRoot:      project,
		ProviderOverride: ProviderOff,
		Command:          "echo vibebox-off",
	})
	if err != nil {
		t.Fatalf("exec off: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("unexpected exit code: %d", result.ExitCode)
	}
	if !strings.Contains(result.Stdout, "vibebox-off") {
		t.Fatalf("unexpected stdout: %q", result.Stdout)
	}
	if result.Selected != ProviderOff {
		t.Fatalf("unexpected selected provider: %s", result.Selected)
	}
}

func TestSessionLifecycleOff(t *testing.T) {
	t.Parallel()
	svc := NewService()
	project := t.TempDir()

	session, err := svc.StartSession(context.Background(), StartSessionRequest{
		ProjectRoot:      project,
		ProviderOverride: ProviderOff,
		Cwd:              ".",
	})
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	if session.State != SessionStateActive {
		t.Fatalf("expected active session, got %s", session.State)
	}

	execResult, err := svc.ExecInSession(context.Background(), ExecInSessionRequest{
		SessionID: session.ID,
		Command:   "echo session-ok",
	})
	if err != nil {
		t.Fatalf("exec in session: %v", err)
	}
	if execResult.ExitCode != 0 {
		t.Fatalf("unexpected exit code: %d", execResult.ExitCode)
	}
	if !strings.Contains(execResult.Stdout, "session-ok") {
		t.Fatalf("unexpected stdout: %q", execResult.Stdout)
	}

	if err := svc.StopSession(context.Background(), StopSessionRequest{SessionID: session.ID}); err != nil {
		t.Fatalf("stop session: %v", err)
	}
	state, err := svc.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if state.State != SessionStateStopped {
		t.Fatalf("expected stopped state, got %s", state.State)
	}
}
