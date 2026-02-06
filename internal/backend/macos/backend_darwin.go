//go:build darwin

package macos

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Code-Hex/vz/v3"

	"vibebox/internal/backend"
)

const (
	bootTimeout         = 90 * time.Second
	loginTimeout        = 180 * time.Second
	promptTimeout       = 30 * time.Second
	stopTimeout         = 30 * time.Second
	defaultExecTimeout  = 10 * time.Minute
	shareTag            = "vibebox-shared"
	sharedMountRoot     = "/mnt/shared"
	stdoutBeginMarker   = "__VIBEBOX_STDOUT_BEGIN__"
	stdoutEndMarker     = "__VIBEBOX_STDOUT_END__"
	stderrBeginMarker   = "__VIBEBOX_STDERR_BEGIN__"
	stderrEndMarker     = "__VIBEBOX_STDERR_END__"
	virtualizationEntID = "com.apple.security.virtualization"
)

var shellPromptHints = []string{
	"~# ",
	":~# ",
	":/# ",
	"/workspace# ",
	"# ",
}

func parseExitMarker(output, marker string) (int, bool) {
	re := regexp.MustCompile(regexp.QuoteMeta(marker) + `(\d+)`)
	matches := re.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return 0, false
	}
	last := matches[len(matches)-1]
	code, err := strconv.Atoi(last[1])
	if err != nil {
		return 0, false
	}
	return code, true
}

func stripExitMarker(output, marker string) string {
	re := regexp.MustCompile(`(?m)^.*` + regexp.QuoteMeta(marker) + `\d+.*\n?`)
	return re.ReplaceAllString(output, "")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func shellExports(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, "export "+k+"="+shellQuote(env[k])+";")
	}
	return strings.Join(parts, " ") + " "
}

func (b *Backend) Probe(ctx context.Context) backend.ProbeResult {
	if _, err := vz.NewEFIBootLoader(); err != nil {
		switch {
		case errors.Is(err, vz.ErrUnsupportedOSVersion):
			return backend.ProbeResult{
				Available: false,
				Reason:    "apple-vm requires macOS 13+ for EFI boot support",
				FixHints:  []string{"upgrade macOS to 13 or newer", "or use provider=docker/off"},
			}
		case errors.Is(err, vz.ErrBuildTargetOSVersion):
			return backend.ProbeResult{
				Available: false,
				Reason:    "apple-vm APIs are disabled in current build target",
				FixHints:  []string{"rebuild vibebox with newer Xcode SDK", "or use provider=docker/off"},
			}
		default:
			return backend.ProbeResult{
				Available: false,
				Reason:    fmt.Sprintf("failed to initialize virtualization framework: %v", err),
				FixHints:  []string{"verify macOS virtualization support", "or use provider=docker/off"},
			}
		}
	}

	if _, err := exec.LookPath("codesign"); err != nil {
		return backend.ProbeResult{
			Available: false,
			Reason:    "codesign command not found",
			FixHints:  []string{"install Xcode command line tools"},
		}
	}

	exe, err := os.Executable()
	if err != nil {
		return backend.ProbeResult{
			Available: false,
			Reason:    fmt.Sprintf("failed to resolve executable path: %v", err),
			FixHints:  []string{"run vibebox from a regular executable path"},
		}
	}
	entitled, err := hasVirtualizationEntitlement(ctx, exe)
	if err != nil {
		return backend.ProbeResult{
			Available: false,
			Reason:    fmt.Sprintf("failed to inspect executable entitlements: %v", err),
			FixHints: []string{
				"run `codesign -d --entitlements - --xml <vibebox-binary>` manually",
				"or use provider=docker/off",
			},
		}
	}
	if !entitled {
		return backend.ProbeResult{
			Available: false,
			Reason:    "vibebox binary is missing virtualization entitlement",
			FixHints: []string{
				"sign the vibebox binary with com.apple.security.virtualization entitlement",
				"or use provider=docker/off",
			},
		}
	}

	return backend.ProbeResult{Available: true}
}

func (b *Backend) Start(ctx context.Context, spec backend.RuntimeSpec) error {
	stdin := spec.IO.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	stdout := spec.IO.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}

	vm, err := newVMRuntime(spec, stdout)
	if err != nil {
		return err
	}
	defer func() {
		_ = vm.Close()
	}()

	if err := vm.Start(ctx); err != nil {
		return err
	}
	if err := vm.Bootstrap(ctx); err != nil {
		_ = vm.TryStop(context.Background())
		return err
	}
	workspaceGuest := workspaceGuestFromSpec(spec)
	if err := vm.SendLine("cd " + shellQuote(workspaceGuest)); err != nil {
		_ = vm.TryStop(context.Background())
		return err
	}
	if _, err := vm.WaitForAny(ctx, shellPromptHints, promptTimeout); err != nil {
		_ = vm.TryStop(context.Background())
		return err
	}

	copyErr := make(chan error, 1)
	go func() {
		_, err := io.Copy(vm.InputWriter(), stdin)
		if err != nil && !errors.Is(err, os.ErrClosed) {
			copyErr <- err
			return
		}
		copyErr <- nil
	}()

	waitErr := make(chan error, 1)
	go func() {
		waitErr <- vm.WaitForState(ctx, vz.VirtualMachineStateStopped, 0)
	}()

	for {
		select {
		case err := <-waitErr:
			return err
		case err := <-copyErr:
			if err != nil {
				_ = vm.TryStop(context.Background())
				return err
			}
			// Host stdin ended; terminate the guest shell session.
			_ = vm.SendLine("exit")
			copyErr = nil
		case <-ctx.Done():
			_ = vm.TryStop(context.Background())
			return ctx.Err()
		}
	}
}

func (b *Backend) Exec(ctx context.Context, spec backend.RuntimeSpec, req backend.ExecRequest) (backend.ExecResult, error) {
	workspaceGuest := workspaceGuestFromSpec(spec)
	if req.Cwd != "" && !strings.HasPrefix(req.Cwd, "/") {
		projectGuest, ok := projectRootGuestFromSpec(spec)
		if !ok {
			return backend.ExecResult{}, fmt.Errorf("relative cwd requires a mount for project root %s", spec.ProjectRoot)
		}
		workspaceGuest = projectGuest
	}
	guestCwd, err := resolveVMGuestCwd(spec.ProjectRoot, req.Cwd, workspaceGuest)
	if err != nil {
		return backend.ExecResult{}, err
	}

	vm, err := newVMRuntime(spec, nil)
	if err != nil {
		return backend.ExecResult{}, err
	}
	defer func() {
		_ = vm.Close()
	}()

	if err := vm.Start(ctx); err != nil {
		return backend.ExecResult{}, err
	}
	if err := vm.Bootstrap(ctx); err != nil {
		_ = vm.TryStop(context.Background())
		return backend.ExecResult{}, err
	}

	script := buildExecScript(guestCwd, req)
	if err := vm.SendLine(script); err != nil {
		_ = vm.TryStop(context.Background())
		return backend.ExecResult{}, err
	}

	wait := req.Timeout
	if wait <= 0 {
		wait = defaultExecTimeout
	}
	if err := vm.WaitForContains(ctx, exitCodeMarker, wait); err != nil {
		_ = vm.TryStop(context.Background())
		return backend.ExecResult{}, err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()
	if err := vm.WaitForState(shutdownCtx, vz.VirtualMachineStateStopped, stopTimeout); err != nil {
		_ = vm.TryStop(context.Background())
	}

	output := vm.Output()
	stdout, stderr, exitCode, ok := parseStructuredExecOutput(output)
	if ok {
		return backend.ExecResult{
			Stdout:   stdout,
			Stderr:   stderr,
			ExitCode: exitCode,
		}, nil
	}

	parsedExit, hasExit := parseExitMarker(output, exitCodeMarker)
	if hasExit {
		return backend.ExecResult{
			Stdout:   stripExitMarker(output, exitCodeMarker),
			Stderr:   "",
			ExitCode: parsedExit,
		}, nil
	}

	return backend.ExecResult{}, fmt.Errorf("apple-vm exec did not produce exit marker; last output: %s", tail(output, 512))
}

func (b *Backend) provisionInstance(ctx context.Context, spec backend.RuntimeSpec) error {
	scriptPath := strings.TrimSpace(spec.Config.VM.ProvisionScript)
	if scriptPath == "" {
		return nil
	}
	if !filepath.IsAbs(scriptPath) {
		scriptPath = filepath.Join(spec.ProjectRoot, scriptPath)
	}
	raw, err := os.ReadFile(scriptPath)
	if err != nil {
		return fmt.Errorf("read provision script %s: %w", scriptPath, err)
	}
	command := buildProvisionCommand(string(raw))

	result, err := b.Exec(ctx, spec, backend.ExecRequest{
		Command: command,
		Cwd:     workspaceGuestFromSpec(spec),
		Timeout: 45 * time.Minute,
	})
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf(
			"provision script exited with code %d (stdout tail=%q, stderr tail=%q)",
			result.ExitCode,
			tail(result.Stdout, 512),
			tail(result.Stderr, 512),
		)
	}
	return nil
}

func hasVirtualizationEntitlement(ctx context.Context, executable string) (bool, error) {
	cmd := exec.CommandContext(ctx, "codesign", "-d", "--entitlements", "-", "--xml", executable)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return false, fmt.Errorf("codesign inspect failed: %s", msg)
	}
	payload := stdout.String() + "\n" + stderr.String()
	return strings.Contains(payload, virtualizationEntID), nil
}

type vmRuntime struct {
	vm            *vz.VirtualMachine
	consoleInput  *os.File
	consoleOutput *os.File
	pipeInRead    *os.File
	pipeOutWrite  *os.File
	readDone      chan struct{}
	readErr       error
	output        bytes.Buffer
	outputMu      sync.Mutex
	tee           io.Writer
	bindings      []shareBinding
}

type shareBinding struct {
	shareName string
	guestPath string
	mode      string
}

func newVMRuntime(spec backend.RuntimeSpec, tee io.Writer) (*vmRuntime, error) {
	consoleInRead, consoleInWrite, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	consoleOutRead, consoleOutWrite, err := os.Pipe()
	if err != nil {
		_ = consoleInRead.Close()
		_ = consoleInWrite.Close()
		return nil, err
	}

	config, bindings, err := buildVMConfiguration(spec, consoleInRead, consoleOutWrite)
	if err != nil {
		_ = consoleInRead.Close()
		_ = consoleInWrite.Close()
		_ = consoleOutRead.Close()
		_ = consoleOutWrite.Close()
		return nil, err
	}

	vm, err := vz.NewVirtualMachine(config)
	if err != nil {
		_ = consoleInRead.Close()
		_ = consoleInWrite.Close()
		_ = consoleOutRead.Close()
		_ = consoleOutWrite.Close()
		return nil, fmt.Errorf("create virtual machine: %w", err)
	}

	r := &vmRuntime{
		vm:            vm,
		consoleInput:  consoleInWrite,
		consoleOutput: consoleOutRead,
		pipeInRead:    consoleInRead,
		pipeOutWrite:  consoleOutWrite,
		readDone:      make(chan struct{}),
		tee:           tee,
		bindings:      bindings,
	}
	go r.readConsoleLoop()
	return r, nil
}

func buildVMConfiguration(spec backend.RuntimeSpec, serialRead, serialWrite *os.File) (*vz.VirtualMachineConfiguration, []shareBinding, error) {
	varStorePath := filepath.Join(filepath.Dir(spec.InstanceRaw), "efi.varstore")
	if err := os.MkdirAll(filepath.Dir(varStorePath), 0o755); err != nil {
		return nil, nil, err
	}

	varStore, err := newOrLoadEFIVariableStore(varStorePath)
	if err != nil {
		return nil, nil, fmt.Errorf("init EFI variable store: %w", err)
	}
	bootLoader, err := vz.NewEFIBootLoader(vz.WithEFIVariableStore(varStore))
	if err != nil {
		return nil, nil, fmt.Errorf("create EFI boot loader: %w", err)
	}

	memBytes := uint64(spec.Config.VM.RAMMB) * 1024 * 1024
	config, err := vz.NewVirtualMachineConfiguration(bootLoader, uint(spec.Config.VM.CPUs), memBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("create VM configuration: %w", err)
	}

	natAttachment, err := vz.NewNATNetworkDeviceAttachment()
	if err != nil {
		return nil, nil, fmt.Errorf("create NAT network attachment: %w", err)
	}
	netDev, err := vz.NewVirtioNetworkDeviceConfiguration(natAttachment)
	if err != nil {
		return nil, nil, fmt.Errorf("create network device config: %w", err)
	}
	if mac, macErr := vz.NewRandomLocallyAdministeredMACAddress(); macErr == nil {
		netDev.SetMACAddress(mac)
	}
	config.SetNetworkDevicesVirtualMachineConfiguration([]*vz.VirtioNetworkDeviceConfiguration{netDev})

	entropy, err := vz.NewVirtioEntropyDeviceConfiguration()
	if err != nil {
		return nil, nil, fmt.Errorf("create entropy device config: %w", err)
	}
	config.SetEntropyDevicesVirtualMachineConfiguration([]*vz.VirtioEntropyDeviceConfiguration{entropy})

	storageAttachment, err := vz.NewDiskImageStorageDeviceAttachment(spec.InstanceRaw, false)
	if err != nil {
		return nil, nil, fmt.Errorf("attach instance disk: %w", err)
	}
	block, err := vz.NewVirtioBlockDeviceConfiguration(storageAttachment)
	if err != nil {
		return nil, nil, fmt.Errorf("create block device config: %w", err)
	}
	config.SetStorageDevicesVirtualMachineConfiguration([]vz.StorageDeviceConfiguration{block})

	shares, bindings, err := buildShares(spec)
	if err != nil {
		return nil, nil, err
	}
	multiShare, err := vz.NewMultipleDirectoryShare(shares)
	if err != nil {
		return nil, nil, fmt.Errorf("create directory share map: %w", err)
	}
	fsDev, err := vz.NewVirtioFileSystemDeviceConfiguration(shareTag)
	if err != nil {
		return nil, nil, fmt.Errorf("create virtiofs config: %w", err)
	}
	fsDev.SetDirectoryShare(multiShare)
	config.SetDirectorySharingDevicesVirtualMachineConfiguration([]vz.DirectorySharingDeviceConfiguration{fsDev})

	serialAttachment, err := vz.NewFileHandleSerialPortAttachment(serialRead, serialWrite)
	if err != nil {
		return nil, nil, fmt.Errorf("create serial attachment: %w", err)
	}
	consolePort, err := vz.NewVirtioConsoleDeviceSerialPortConfiguration(serialAttachment)
	if err != nil {
		return nil, nil, fmt.Errorf("create serial console config: %w", err)
	}
	config.SetSerialPortsVirtualMachineConfiguration([]*vz.VirtioConsoleDeviceSerialPortConfiguration{consolePort})

	valid, err := config.Validate()
	if err != nil {
		return nil, nil, fmt.Errorf("validate VM configuration: %w", err)
	}
	if !valid {
		return nil, nil, fmt.Errorf("validate VM configuration: invalid")
	}
	return config, bindings, nil
}

func newOrLoadEFIVariableStore(path string) (*vz.EFIVariableStore, error) {
	if _, err := os.Stat(path); err == nil {
		return vz.NewEFIVariableStore(path)
	}
	return vz.NewEFIVariableStore(path, vz.WithCreatingEFIVariableStore())
}

func (r *vmRuntime) readConsoleLoop() {
	defer close(r.readDone)
	buf := make([]byte, 4096)
	for {
		n, err := r.consoleOutput.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if r.tee != nil {
				_, _ = r.tee.Write(chunk)
			}
			r.outputMu.Lock()
			_, _ = r.output.Write(chunk)
			r.outputMu.Unlock()
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) {
				r.readErr = err
			}
			return
		}
	}
}

func (r *vmRuntime) Start(ctx context.Context) error {
	if !r.vm.CanStart() {
		return fmt.Errorf("virtual machine cannot start in current state: %v", r.vm.State())
	}
	if err := r.vm.Start(); err != nil {
		return fmt.Errorf("start VM: %w", err)
	}
	return r.WaitForState(ctx, vz.VirtualMachineStateRunning, bootTimeout)
}

func (r *vmRuntime) WaitForState(ctx context.Context, want vz.VirtualMachineState, timeout time.Duration) error {
	if got := r.vm.State(); got == want {
		return nil
	}

	stateChanged := r.vm.StateChangedNotify()
	var timeoutC <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		timeoutC = timer.C
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeoutC:
			return fmt.Errorf("timed out waiting VM state %v (current %v)", want, r.vm.State())
		case got, ok := <-stateChanged:
			if !ok {
				return fmt.Errorf("VM state notification closed before reaching %v", want)
			}
			if got == want {
				return nil
			}
			if got == vz.VirtualMachineStateError {
				return fmt.Errorf("VM entered error state")
			}
		}
	}
}

func (r *vmRuntime) Bootstrap(ctx context.Context) error {
	matched, err := r.WaitForAny(ctx, append([]string{"login: "}, shellPromptHints...), loginTimeout)
	if err != nil {
		return fmt.Errorf("wait for login prompt: %w", err)
	}

	if matched == "login: " {
		if err := r.SendLine("root"); err != nil {
			return err
		}
		if _, err := r.WaitForAny(ctx, shellPromptHints, promptTimeout); err != nil {
			return fmt.Errorf("wait for shell prompt after login: %w", err)
		}
	}

	mountCmd := buildGuestMountCommand(r.bindings)
	if err := r.SendLine(mountCmd); err != nil {
		return err
	}
	if _, err := r.WaitForAny(ctx, shellPromptHints, promptTimeout); err != nil {
		return fmt.Errorf("wait for shell prompt after mount: %w", err)
	}
	return nil
}

func (r *vmRuntime) SendLine(line string) error {
	_, err := io.WriteString(r.consoleInput, line+"\n")
	return err
}

func (r *vmRuntime) WaitForContains(ctx context.Context, text string, timeout time.Duration) error {
	return r.waitForMatch(ctx, []string{text}, timeout, false)
}

func (r *vmRuntime) WaitForAny(ctx context.Context, texts []string, timeout time.Duration) (string, error) {
	return r.waitForAnyMatch(ctx, texts, timeout)
}

func (r *vmRuntime) waitForAnyMatch(ctx context.Context, texts []string, timeout time.Duration) (string, error) {
	err := r.waitForMatch(ctx, texts, timeout, true)
	if err != nil {
		return "", err
	}
	out := r.Output()
	for _, text := range texts {
		if strings.Contains(out, text) {
			return text, nil
		}
	}
	return "", fmt.Errorf("internal error: matched text not found")
}

func (r *vmRuntime) waitForMatch(ctx context.Context, texts []string, timeout time.Duration, any bool) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		out := r.Output()
		if any {
			for _, text := range texts {
				if strings.Contains(out, text) {
					return nil
				}
			}
		} else if len(texts) == 1 && strings.Contains(out, texts[0]) {
			return nil
		}

		if timeout > 0 && time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for console output %v", texts)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *vmRuntime) Output() string {
	r.outputMu.Lock()
	defer r.outputMu.Unlock()
	return r.output.String()
}

func (r *vmRuntime) InputWriter() io.Writer {
	return r.consoleInput
}

func (r *vmRuntime) TryStop(ctx context.Context) error {
	if r.vm.State() == vz.VirtualMachineStateStopped {
		return nil
	}
	if r.vm.CanRequestStop() {
		_, _ = r.vm.RequestStop()
	}

	stopCtx := ctx
	if stopCtx == nil {
		stopCtx = context.Background()
	}
	waitCtx, cancel := context.WithTimeout(stopCtx, stopTimeout)
	defer cancel()
	if err := r.WaitForState(waitCtx, vz.VirtualMachineStateStopped, stopTimeout); err == nil {
		return nil
	}

	if r.vm.CanStop() {
		if err := r.vm.Stop(); err == nil {
			_ = r.WaitForState(context.Background(), vz.VirtualMachineStateStopped, stopTimeout)
			return nil
		}
	}
	return nil
}

func (r *vmRuntime) Close() error {
	_ = r.consoleInput.Close()
	_ = r.pipeInRead.Close()
	_ = r.pipeOutWrite.Close()
	_ = r.consoleOutput.Close()
	<-r.readDone
	return nil
}

func buildExecScript(guestCwd string, req backend.ExecRequest) string {
	return fmt.Sprintf(
		"tmp_out=$(mktemp); tmp_err=$(mktemp); (cd %s && %sbash -lc %s) >\"$tmp_out\" 2>\"$tmp_err\"; rc=$?; printf '%s\\n'; cat \"$tmp_out\"; printf '\\n%s\\n'; printf '%s\\n'; cat \"$tmp_err\"; printf '\\n%s\\n'; printf '%s%%s\\n' \"$rc\"; rm -f \"$tmp_out\" \"$tmp_err\"; poweroff",
		shellQuote(guestCwd),
		shellExports(req.Env),
		shellQuote(req.Command),
		stdoutBeginMarker,
		stdoutEndMarker,
		stderrBeginMarker,
		stderrEndMarker,
		exitCodeMarker,
	)
}

func buildProvisionCommand(script string) string {
	delimiter := "__VIBEBOX_PROVISION_EOF__"
	for strings.Contains(script, delimiter) {
		delimiter += "_X"
	}
	var b strings.Builder
	b.WriteString("cat >/tmp/vibebox-provision.sh <<'")
	b.WriteString(delimiter)
	b.WriteString("'\n")
	b.WriteString(script)
	if !strings.HasSuffix(script, "\n") {
		b.WriteString("\n")
	}
	b.WriteString(delimiter)
	b.WriteString("\n")
	b.WriteString("chmod +x /tmp/vibebox-provision.sh\n")
	b.WriteString("VIBEBOX_PROVISION_POWEROFF=0 /bin/bash /tmp/vibebox-provision.sh\n")
	return b.String()
}

func buildShares(spec backend.RuntimeSpec) (map[string]*vz.SharedDirectory, []shareBinding, error) {
	mounts := spec.Config.Mounts
	if len(mounts) == 0 {
		sharedDir, err := vz.NewSharedDirectory(spec.ProjectRoot, false)
		if err != nil {
			return nil, nil, fmt.Errorf("create shared directory %s: %w", spec.ProjectRoot, err)
		}
		shares := map[string]*vz.SharedDirectory{"share0": sharedDir}
		bindings := []shareBinding{{
			shareName: "share0",
			guestPath: workspaceGuestFromSpec(spec),
			mode:      "rw",
		}}
		return shares, bindings, nil
	}

	shares := make(map[string]*vz.SharedDirectory, len(mounts))
	bindings := make([]shareBinding, 0, len(mounts))
	for i, m := range mounts {
		host := m.Host
		guest := m.Guest
		mode := m.Mode
		if host == "" {
			host = spec.ProjectRoot
		}
		if guest == "" {
			guest = workspaceGuestPath
		}
		if mode == "" {
			mode = "rw"
		}
		if !filepath.IsAbs(host) {
			host = filepath.Join(spec.ProjectRoot, host)
		}
		host = filepath.Clean(host)
		guest = filepath.ToSlash(filepath.Clean(guest))
		if !strings.HasPrefix(guest, "/") {
			return nil, nil, fmt.Errorf("mount guest path must be absolute: %s", guest)
		}
		info, err := os.Stat(host)
		if err != nil {
			return nil, nil, fmt.Errorf("mount host path does not exist: %s", host)
		}
		if !info.IsDir() {
			return nil, nil, fmt.Errorf("mount host path is not a directory: %s", host)
		}
		readOnly := mode == "ro"
		sharedDir, err := vz.NewSharedDirectory(host, readOnly)
		if err != nil {
			return nil, nil, fmt.Errorf("create shared directory %s: %w", host, err)
		}
		shareName := fmt.Sprintf("share%d", i)
		shares[shareName] = sharedDir
		bindings = append(bindings, shareBinding{
			shareName: shareName,
			guestPath: guest,
			mode:      mode,
		})
	}
	return shares, bindings, nil
}

func buildGuestMountCommand(bindings []shareBinding) string {
	var b strings.Builder
	b.WriteString("mkdir -p ")
	b.WriteString(shellQuote(sharedMountRoot))
	b.WriteString(" && mount -t virtiofs ")
	b.WriteString(shellQuote(shareTag))
	b.WriteString(" ")
	b.WriteString(shellQuote(sharedMountRoot))
	for _, m := range bindings {
		stagingPath := filepath.ToSlash(filepath.Join(sharedMountRoot, m.shareName))
		b.WriteString(" && mkdir -p ")
		b.WriteString(shellQuote(m.guestPath))
		b.WriteString(" && mount --bind ")
		b.WriteString(shellQuote(stagingPath))
		b.WriteString(" ")
		b.WriteString(shellQuote(m.guestPath))
		if m.mode == "ro" {
			b.WriteString(" && mount -o remount,ro,bind ")
			b.WriteString(shellQuote(m.guestPath))
		}
	}
	return b.String()
}

func parseStructuredExecOutput(output string) (stdout string, stderr string, exitCode int, ok bool) {
	exitCode, ok = parseExitMarker(output, exitCodeMarker)
	if !ok {
		return "", "", 0, false
	}
	stdout, ok = extractBetweenMarkers(output, stdoutBeginMarker, stdoutEndMarker)
	if !ok {
		return "", "", 0, false
	}
	stderr, ok = extractBetweenMarkers(output, stderrBeginMarker, stderrEndMarker)
	if !ok {
		return "", "", 0, false
	}
	return strings.TrimPrefix(stdout, "\n"), strings.TrimPrefix(stderr, "\n"), exitCode, true
}

func extractBetweenMarkers(output, begin, end string) (string, bool) {
	start := strings.LastIndex(output, begin)
	if start < 0 {
		return "", false
	}
	start += len(begin)
	remaining := output[start:]
	finish := strings.Index(remaining, end)
	if finish < 0 {
		return "", false
	}
	return remaining[:finish], true
}

func tail(s string, max int) string {
	clean := strings.TrimSpace(stripANSI(s))
	if len(clean) <= max {
		return clean
	}
	return clean[len(clean)-max:]
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}
