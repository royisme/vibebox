package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"vibebox/internal/app"
	"vibebox/internal/config"
	sdk "vibebox/pkg/vibebox"
)

func main() {
	exitCode, err := runWithIO(context.Background(), os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Error:", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

func runWithIO(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) (int, error) {
	a := app.New(stdout, stderr)
	svc := sdk.NewService()
	if len(args) == 0 {
		printRootHelp(stdout)
		return 0, nil
	}

	switch args[0] {
	case "init":
		fs := flag.NewFlagSet("init", flag.ContinueOnError)
		fs.SetOutput(stderr)
		var nonInteractive bool
		var imageID string
		var provider string
		var cpus int
		var ramMB int
		var diskGB int
		fs.BoolVar(&nonInteractive, "non-interactive", false, "disable TUI wizard")
		fs.StringVar(&imageID, "image-id", "", "official image id")
		fs.StringVar(&provider, "provider", string(config.ProviderAuto), "provider: off|apple-vm|docker|auto")
		fs.IntVar(&cpus, "cpus", 2, "vm CPU count")
		fs.IntVar(&ramMB, "ram-mb", 2048, "vm memory in MiB")
		fs.IntVar(&diskGB, "disk-gb", 20, "vm disk in GiB")
		if err := fs.Parse(args[1:]); err != nil {
			return 1, err
		}
		return 0, a.Init(ctx, app.InitOptions{
			NonInteractive: nonInteractive,
			ImageID:        imageID,
			Provider:       config.Provider(provider),
			CPUs:           cpus,
			RAMMB:          ramMB,
			DiskGB:         diskGB,
		})
	case "up":
		fs := flag.NewFlagSet("up", flag.ContinueOnError)
		fs.SetOutput(stderr)
		var provider string
		fs.StringVar(&provider, "provider", "", "override provider: off|apple-vm|docker|auto")
		if err := fs.Parse(args[1:]); err != nil {
			return 1, err
		}
		return 0, a.Up(ctx, app.UpOptions{Provider: config.Provider(provider)})
	case "images":
		if len(args) == 1 {
			printImagesHelp(stdout)
			return 0, nil
		}
		sub := args[1]
		switch sub {
		case "list":
			return 0, a.ImagesList()
		case "upgrade":
			fs := flag.NewFlagSet("images upgrade", flag.ContinueOnError)
			fs.SetOutput(stderr)
			var imageID string
			fs.StringVar(&imageID, "image-id", "", "image id to refresh")
			if err := fs.Parse(args[2:]); err != nil {
				return 1, err
			}
			return 0, a.ImagesUpgrade(ctx, app.UpgradeOptions{ImageID: imageID})
		default:
			printImagesHelp(stdout)
			return 1, fmt.Errorf("unknown images subcommand: %s", sub)
		}
	case "probe":
		return runProbe(ctx, svc, args[1:], stdout, stderr)
	case "exec":
		return runExec(ctx, svc, args[1:], stdout, stderr)
	case "help", "--help", "-h":
		printRootHelp(stdout)
		return 0, nil
	default:
		printRootHelp(stdout)
		return 1, fmt.Errorf("unknown command: %s", args[0])
	}
}

type probeJSONResponse struct {
	OK           bool                             `json:"ok"`
	Error        string                           `json:"error,omitempty"`
	Selected     string                           `json:"selected"`
	WasFallback  bool                             `json:"wasFallback"`
	FallbackFrom string                           `json:"fallbackFrom"`
	Diagnostics  map[string]sdk.BackendDiagnostic `json:"diagnostics"`
}

type execJSONResponse struct {
	OK          bool                             `json:"ok"`
	Error       string                           `json:"error,omitempty"`
	Selected    string                           `json:"selected"`
	ExitCode    int                              `json:"exitCode"`
	Stdout      string                           `json:"stdout"`
	Stderr      string                           `json:"stderr"`
	Diagnostics map[string]sdk.BackendDiagnostic `json:"diagnostics"`
}

func runProbe(ctx context.Context, svc *sdk.Service, args []string, stdout io.Writer, stderr io.Writer) (int, error) {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var provider string
	var projectRoot string
	var jsonMode bool
	fs.StringVar(&provider, "provider", string(sdk.ProviderAuto), "provider: off|apple-vm|docker|auto")
	fs.StringVar(&projectRoot, "project-root", "", "project root path (optional)")
	fs.BoolVar(&jsonMode, "json", false, "output machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return 1, err
	}

	if projectRoot != "" {
		if _, err := os.Stat(projectRoot); err != nil {
			if jsonMode {
				_ = writeJSON(stdout, probeJSONResponse{OK: false, Error: err.Error(), Diagnostics: map[string]sdk.BackendDiagnostic{}, Selected: "", WasFallback: false, FallbackFrom: ""})
				return 1, nil
			}
			return 1, err
		}
	}

	result, err := svc.Probe(ctx, sdk.Provider(provider))
	if jsonMode {
		resp := probeJSONResponse{
			OK:           err == nil,
			Selected:     string(result.Selected),
			WasFallback:  result.WasFallback,
			FallbackFrom: result.FallbackFrom,
			Diagnostics:  normalizeDiagnostics(result.Diagnostics),
		}
		if resp.Diagnostics == nil {
			resp.Diagnostics = map[string]sdk.BackendDiagnostic{}
		}
		if err != nil {
			resp.Error = err.Error()
			resp.Selected = ""
		}
		if writeErr := writeJSON(stdout, resp); writeErr != nil {
			return 1, writeErr
		}
		if err != nil {
			return 1, nil
		}
		return 0, nil
	}

	if err != nil {
		return 1, err
	}
	_, _ = fmt.Fprintf(stdout, "selected=%s fallback=%v from=%s\n", result.Selected, result.WasFallback, result.FallbackFrom)
	for name, d := range result.Diagnostics {
		_, _ = fmt.Fprintf(stdout, "%s available=%v reason=%q hints=%v\n", name, d.Available, d.Reason, d.FixHints)
	}
	return 0, nil
}

type envValues []string

func (e *envValues) String() string {
	return strings.Join(*e, ",")
}

func (e *envValues) Set(v string) error {
	*e = append(*e, v)
	return nil
}

func parseEnv(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := map[string]string{}
	for _, v := range values {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return nil, fmt.Errorf("invalid env value: %q (expected KEY=VALUE)", v)
		}
		out[parts[0]] = parts[1]
	}
	return out, nil
}

func runExec(ctx context.Context, svc *sdk.Service, args []string, stdout io.Writer, stderr io.Writer) (int, error) {
	fs := flag.NewFlagSet("exec", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var provider string
	var projectRoot string
	var command string
	var cwd string
	var timeoutSeconds int
	var jsonMode bool
	var envs envValues
	fs.StringVar(&provider, "provider", string(sdk.ProviderAuto), "provider: off|apple-vm|docker|auto")
	fs.StringVar(&projectRoot, "project-root", "", "project root path (optional)")
	fs.StringVar(&command, "command", "", "command to execute (required)")
	fs.StringVar(&cwd, "cwd", "", "working directory inside sandbox")
	fs.IntVar(&timeoutSeconds, "timeout-seconds", 0, "timeout in seconds")
	fs.Var(&envs, "env", "environment variable KEY=VALUE (repeatable)")
	fs.BoolVar(&jsonMode, "json", false, "output machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return 1, err
	}

	envMap, err := parseEnv(envs)
	if err != nil {
		if jsonMode {
			_ = writeJSON(stdout, execJSONResponse{OK: false, Error: err.Error(), Selected: "", ExitCode: 1, Stdout: "", Stderr: "", Diagnostics: map[string]sdk.BackendDiagnostic{}})
			return 1, nil
		}
		return 1, err
	}

	if command == "" {
		err := fmt.Errorf("--command is required")
		if jsonMode {
			_ = writeJSON(stdout, execJSONResponse{OK: false, Error: err.Error(), Selected: "", ExitCode: 1, Stdout: "", Stderr: "", Diagnostics: map[string]sdk.BackendDiagnostic{}})
			return 1, nil
		}
		return 1, err
	}

	result, execErr := svc.Exec(ctx, sdk.ExecRequest{
		ProjectRoot:      projectRoot,
		ProviderOverride: sdk.Provider(provider),
		Command:          command,
		Cwd:              cwd,
		Env:              envMap,
		TimeoutSeconds:   timeoutSeconds,
	})

	if jsonMode {
		diagnostics := normalizeDiagnostics(result.Diagnostics)
		if diagnostics == nil {
			diagnostics = map[string]sdk.BackendDiagnostic{}
		}
		if execErr != nil {
			probeResult, _ := svc.Probe(ctx, sdk.Provider(provider))
			if len(diagnostics) == 0 && probeResult.Diagnostics != nil {
				diagnostics = normalizeDiagnostics(probeResult.Diagnostics)
			}
			resp := execJSONResponse{
				OK:          false,
				Error:       execErr.Error(),
				Selected:    "",
				ExitCode:    1,
				Stdout:      "",
				Stderr:      "",
				Diagnostics: diagnostics,
			}
			if err := writeJSON(stdout, resp); err != nil {
				return 1, err
			}
			return 1, nil
		}
		resp := execJSONResponse{
			OK:          true,
			Selected:    string(result.Selected),
			ExitCode:    result.ExitCode,
			Stdout:      result.Stdout,
			Stderr:      result.Stderr,
			Diagnostics: diagnostics,
		}
		if err := writeJSON(stdout, resp); err != nil {
			return 1, err
		}
		return result.ExitCode, nil
	}

	if execErr != nil {
		return 1, execErr
	}
	if result.Stdout != "" {
		_, _ = fmt.Fprint(stdout, result.Stdout)
	}
	if result.Stderr != "" {
		_, _ = fmt.Fprint(stderr, result.Stderr)
	}
	return result.ExitCode, nil
}

func writeJSON(w io.Writer, payload any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(payload)
}

func normalizeDiagnostics(in map[string]sdk.BackendDiagnostic) map[string]sdk.BackendDiagnostic {
	if in == nil {
		return nil
	}
	out := make(map[string]sdk.BackendDiagnostic, len(in))
	for k, d := range in {
		if d.FixHints == nil {
			d.FixHints = []string{}
		}
		out[k] = d
	}
	return out
}

func printRootHelp(w io.Writer) {
	_, _ = fmt.Fprint(w, `vibebox - sandbox launcher for LLM agents

Usage:
  vibebox init [flags]           Initialize project sandbox
  vibebox up [--provider ...]    Start sandbox shell
  vibebox probe [--json]         Probe backend availability and selection
  vibebox exec [--json]          Execute one command non-interactively
  vibebox images list            List official VM images
  vibebox images upgrade         Refresh/download an image

Common flags:
  --provider off|apple-vm|docker|auto
`)
}

func printImagesHelp(w io.Writer) {
	_, _ = fmt.Fprint(w, `vibebox images commands:
  vibebox images list
  vibebox images upgrade [--image-id <id>]`)
}
