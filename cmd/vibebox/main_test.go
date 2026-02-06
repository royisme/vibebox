package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestProbeJSON(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	var errBuf bytes.Buffer

	code, err := runWithIO(context.Background(), []string{"probe", "--json", "--provider", "off"}, &out, &errBuf)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected code 0, got %d", code)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v\noutput=%q", err, out.String())
	}
	if ok, _ := payload["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", payload["ok"])
	}
	if selected, _ := payload["selected"].(string); selected != "off" {
		t.Fatalf("expected selected=off, got %q", selected)
	}
}

func TestExecJSONOff(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	var out bytes.Buffer
	var errBuf bytes.Buffer

	args := []string{"exec", "--json", "--provider", "off", "--project-root", project, "--command", "echo vibebox"}
	code, err := runWithIO(context.Background(), args, &out, &errBuf)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected code 0, got %d; stderr=%q", code, errBuf.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v\noutput=%q", err, out.String())
	}
	if ok, _ := payload["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v", payload["ok"])
	}
	if exitCode, _ := payload["exitCode"].(float64); int(exitCode) != 0 {
		t.Fatalf("expected exitCode=0, got %v", payload["exitCode"])
	}
}

func TestExecJSONMissingCommand(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	var errBuf bytes.Buffer

	code, err := runWithIO(context.Background(), []string{"exec", "--json", "--provider", "off"}, &out, &errBuf)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code == 0 {
		t.Fatalf("expected non-zero code")
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v\noutput=%q", err, out.String())
	}
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false")
	}
	if _, exists := payload["error"]; !exists {
		t.Fatalf("expected error field")
	}
}

func TestParseMountSpecs(t *testing.T) {
	t.Parallel()

	mounts, err := parseMountSpecs([]string{"./workspace:/workspace", "../cache:/cache:ro"})
	if err != nil {
		t.Fatalf("parse mounts: %v", err)
	}
	if len(mounts) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(mounts))
	}
	if mounts[0].Mode != "rw" {
		t.Fatalf("expected default rw mode, got %s", mounts[0].Mode)
	}
	if mounts[1].Mode != "ro" {
		t.Fatalf("expected ro mode, got %s", mounts[1].Mode)
	}
}

func TestParseMountSpecsInvalid(t *testing.T) {
	t.Parallel()
	if _, err := parseMountSpecs([]string{"bad"}); err == nil {
		t.Fatalf("expected error for invalid mount format")
	}
	if _, err := parseMountSpecs([]string{"a:b:xx"}); err == nil {
		t.Fatalf("expected error for invalid mount mode")
	}
}
