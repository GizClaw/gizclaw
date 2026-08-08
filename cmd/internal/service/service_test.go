package service

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	kservice "github.com/kardianos/service"
)

func TestServiceLabelConstant(t *testing.T) {
	if serviceLabel != "com.gizclaw.serve" {
		t.Fatalf("serviceLabel = %q", serviceLabel)
	}
}

func TestNewServiceConfigUsesServeForceCommand(t *testing.T) {
	spec := Spec{
		WorkspaceRoot: "/tmp/workspace",
		Executable:    "/usr/local/bin/gizclaw",
		Label:         "com.gizclaw.serve.test",
	}
	cfg := newServiceConfig(spec)
	if cfg.Name != spec.Label {
		t.Fatalf("cfg.Name = %q", cfg.Name)
	}
	if cfg.Executable != spec.Executable {
		t.Fatalf("cfg.Executable = %q", cfg.Executable)
	}
	if cfg.WorkingDirectory != spec.WorkspaceRoot {
		t.Fatalf("cfg.WorkingDirectory = %q", cfg.WorkingDirectory)
	}
	wantArgs := []string{"serve", "--force", spec.WorkspaceRoot}
	if !equalStrings(cfg.Arguments, wantArgs) {
		t.Fatalf("cfg.Arguments = %#v, want %#v", cfg.Arguments, wantArgs)
	}
	if keepAlive, ok := cfg.Option["KeepAlive"].(bool); !ok || !keepAlive {
		t.Fatalf("cfg.Option[KeepAlive] = %#v", cfg.Option["KeepAlive"])
	}
	if runAtLoad, ok := cfg.Option["RunAtLoad"].(bool); !ok || runAtLoad {
		t.Fatalf("cfg.Option[RunAtLoad] = %#v", cfg.Option["RunAtLoad"])
	}
	if _, ok := cfg.Option["UserService"]; ok {
		t.Fatalf("cfg.Option[UserService] = %#v, want absent for system service", cfg.Option["UserService"])
	}
}

func TestNoopProgramStartStop(t *testing.T) {
	var p noopProgram
	if err := p.Start(nil); err != nil {
		t.Fatalf("noopProgram.Start() error = %v", err)
	}
	if err := p.Stop(nil); err != nil {
		t.Fatalf("noopProgram.Stop() error = %v", err)
	}
}

func TestServiceSpecPropagatesExecutableError(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	executablePath = func() (string, error) { return "", errors.New("boom") }
	if _, err := serviceSpec(t.TempDir()); err == nil {
		t.Fatal("serviceSpec() should reject executable path errors")
	}
}

func TestInstallWritesMarkerAndRecord(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	workspace := t.TempDir()
	fake := &fakeManagedService{statusErr: kservice.ErrNotInstalled}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}
	if err := Install(workspace); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !fake.installCalled {
		t.Fatal("Install() should call managed service Install")
	}
	managed, err := WorkspaceManaged(workspace)
	if err != nil {
		t.Fatalf("WorkspaceManaged() error = %v", err)
	}
	if !managed {
		t.Fatal("workspace should be marked managed after install")
	}
	record, err := readInstallRecord()
	if err != nil {
		t.Fatalf("readInstallRecord() error = %v", err)
	}
	if record.WorkspaceRoot != workspace {
		t.Fatalf("record.WorkspaceRoot = %q", record.WorkspaceRoot)
	}
}

func TestInstallRejectsAlreadyInstalledService(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	fake := &fakeManagedService{status: kservice.StatusStopped}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}

	if err := Install(t.TempDir()); err == nil {
		t.Fatal("Install() should reject an already installed service")
	}
}

func TestInstallRejectsExistingInstallRecord(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	workspace := t.TempDir()
	if err := writeInstallRecord(Spec{WorkspaceRoot: workspace}); err != nil {
		t.Fatalf("writeInstallRecord() error = %v", err)
	}
	fake := &fakeManagedService{statusErr: kservice.ErrNotInstalled}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}

	if err := Install(workspace); err == nil {
		t.Fatal("Install() should reject an existing install record")
	}
	if fake.installCalled {
		t.Fatal("Install() should not install when an install record already exists")
	}
}

func TestInstallPropagatesServiceStatusError(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	fake := &fakeManagedService{statusErr: errors.New("boom")}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}

	if err := Install(t.TempDir()); err == nil {
		t.Fatal("Install() should propagate service status errors")
	}
}

func TestInstallPropagatesServiceCreateError(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return nil, errors.New("boom")
	}

	if err := Install(t.TempDir()); err == nil {
		t.Fatal("Install() should propagate service creation errors")
	}
}

func TestInstallPropagatesInstallError(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	fake := &fakeManagedService{statusErr: kservice.ErrNotInstalled, installErr: errors.New("boom")}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}

	if err := Install(t.TempDir()); err == nil {
		t.Fatal("Install() should propagate install errors")
	}
}

func TestNewServiceReturnsManagedService(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	fake := &fakeManagedService{}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		if spec.WorkspaceRoot == "" {
			t.Fatal("NewService() should resolve workspace root")
		}
		return fake, nil
	}

	svc, err := NewService(t.TempDir(), noopProgram{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if svc != fake {
		t.Fatal("NewService() should return managed service")
	}
}

func TestNewServicePropagatesServiceCreateError(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return nil, errors.New("boom")
	}

	if _, err := NewService(t.TempDir(), noopProgram{}); err == nil {
		t.Fatal("NewService() should propagate service creation errors")
	}
}

func TestStartStartsInstalledService(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	workspace := t.TempDir()
	if err := writeInstallRecord(Spec{WorkspaceRoot: workspace}); err != nil {
		t.Fatalf("writeInstallRecord() error = %v", err)
	}
	fake := &fakeManagedService{status: kservice.StatusStopped}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}

	if err := Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !fake.startCalled {
		t.Fatal("Start() should start installed service")
	}
}

func TestStartRejectsMissingService(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	workspace := t.TempDir()
	if err := writeInstallRecord(Spec{WorkspaceRoot: workspace}); err != nil {
		t.Fatalf("writeInstallRecord() error = %v", err)
	}
	fake := &fakeManagedService{statusErr: kservice.ErrNotInstalled}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}

	if err := Start(); err == nil {
		t.Fatal("Start() should reject a missing service")
	}
}

func TestStartPropagatesStartError(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	workspace := t.TempDir()
	if err := writeInstallRecord(Spec{WorkspaceRoot: workspace}); err != nil {
		t.Fatalf("writeInstallRecord() error = %v", err)
	}
	fake := &fakeManagedService{status: kservice.StatusStopped, startErr: errors.New("boom")}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}

	if err := Start(); err == nil {
		t.Fatal("Start() should propagate start errors")
	}
}

func TestLifecycleCommandsRequireInstallRecord(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	for name, run := range map[string]func() error{
		"start":     Start,
		"restart":   Restart,
		"stop":      Stop,
		"uninstall": Uninstall,
	} {
		t.Run(name, func(t *testing.T) {
			if err := run(); err == nil {
				t.Fatalf("%s should reject missing install record", name)
			}
		})
	}
}

func TestLifecycleCommandsPropagateServiceCreateError(t *testing.T) {
	for name, run := range map[string]func() error{
		"start":     Start,
		"restart":   Restart,
		"stop":      Stop,
		"uninstall": Uninstall,
	} {
		t.Run(name, func(t *testing.T) {
			restore := stubPaths(t)
			defer restore()

			workspace := t.TempDir()
			if err := writeInstallRecord(Spec{WorkspaceRoot: workspace}); err != nil {
				t.Fatalf("writeInstallRecord() error = %v", err)
			}
			newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
				return nil, errors.New("boom")
			}

			if err := run(); err == nil {
				t.Fatalf("%s should propagate service creation errors", name)
			}
		})
	}
}

func TestRestartRestartsInstalledService(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	workspace := t.TempDir()
	if err := writeInstallRecord(Spec{WorkspaceRoot: workspace}); err != nil {
		t.Fatalf("writeInstallRecord() error = %v", err)
	}
	fake := &fakeManagedService{status: kservice.StatusRunning}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}

	if err := Restart(); err != nil {
		t.Fatalf("Restart() error = %v", err)
	}
	if !fake.restartCalled {
		t.Fatal("Restart() should restart installed service")
	}
}

func TestRestartRejectsMissingService(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	workspace := t.TempDir()
	if err := writeInstallRecord(Spec{WorkspaceRoot: workspace}); err != nil {
		t.Fatalf("writeInstallRecord() error = %v", err)
	}
	fake := &fakeManagedService{statusErr: kservice.ErrNotInstalled}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}

	if err := Restart(); err == nil {
		t.Fatal("Restart() should reject a missing service")
	}
}

func TestRestartPropagatesRestartError(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	workspace := t.TempDir()
	if err := writeInstallRecord(Spec{WorkspaceRoot: workspace}); err != nil {
		t.Fatalf("writeInstallRecord() error = %v", err)
	}
	fake := &fakeManagedService{status: kservice.StatusRunning, restartErr: errors.New("boom")}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}

	if err := Restart(); err == nil {
		t.Fatal("Restart() should propagate restart errors")
	}
}

func TestStopStopsRunningService(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	workspace := t.TempDir()
	if err := writeInstallRecord(Spec{WorkspaceRoot: workspace}); err != nil {
		t.Fatalf("writeInstallRecord() error = %v", err)
	}
	fake := &fakeManagedService{status: kservice.StatusRunning}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}

	if err := Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if !fake.stopCalled {
		t.Fatal("Stop() should stop a running service")
	}
}

func TestStopIgnoresStoppedService(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	workspace := t.TempDir()
	if err := writeInstallRecord(Spec{WorkspaceRoot: workspace}); err != nil {
		t.Fatalf("writeInstallRecord() error = %v", err)
	}
	fake := &fakeManagedService{status: kservice.StatusStopped}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}

	if err := Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if fake.stopCalled {
		t.Fatal("Stop() should not stop an already stopped service")
	}
}

func TestStopRejectsMissingService(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	workspace := t.TempDir()
	if err := writeInstallRecord(Spec{WorkspaceRoot: workspace}); err != nil {
		t.Fatalf("writeInstallRecord() error = %v", err)
	}
	fake := &fakeManagedService{statusErr: kservice.ErrNotInstalled}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}

	if err := Stop(); err == nil {
		t.Fatal("Stop() should reject a missing service")
	}
}

func TestStopPropagatesStopError(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	workspace := t.TempDir()
	if err := writeInstallRecord(Spec{WorkspaceRoot: workspace}); err != nil {
		t.Fatalf("writeInstallRecord() error = %v", err)
	}
	fake := &fakeManagedService{status: kservice.StatusRunning, stopErr: errors.New("boom")}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}

	if err := Stop(); err == nil {
		t.Fatal("Stop() should propagate stop errors")
	}
}

func TestStatusReturnsNotInstalledWithoutError(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	fake := &fakeManagedService{statusErr: kservice.ErrNotInstalled}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}

	info, err := Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if info.Installed {
		t.Fatal("Status().Installed should be false")
	}
	if info.Running {
		t.Fatal("Status().Running should be false")
	}
	if info.State != "not installed" {
		t.Fatalf("Status().State = %q", info.State)
	}
	if info.ServiceName != serviceLabel {
		t.Fatalf("Status().ServiceName = %q", info.ServiceName)
	}
}

func TestStatusReturnsRunningInfo(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	workspace := t.TempDir()
	if err := writeInstallRecord(Spec{WorkspaceRoot: workspace}); err != nil {
		t.Fatalf("writeInstallRecord() error = %v", err)
	}

	fake := &fakeManagedService{status: kservice.StatusRunning}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}

	info, err := Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !info.Installed {
		t.Fatal("Status().Installed should be true")
	}
	if !info.Running {
		t.Fatal("Status().Running should be true")
	}
	if info.State != "running" {
		t.Fatalf("Status().State = %q", info.State)
	}
	if info.WorkspaceRoot != workspace {
		t.Fatalf("Status().WorkspaceRoot = %q", info.WorkspaceRoot)
	}
}

func TestStatusReturnsStoppedInfo(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	workspace := t.TempDir()
	if err := writeInstallRecord(Spec{WorkspaceRoot: workspace}); err != nil {
		t.Fatalf("writeInstallRecord() error = %v", err)
	}

	fake := &fakeManagedService{status: kservice.StatusStopped}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}

	info, err := Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !info.Installed {
		t.Fatal("Status().Installed should be true")
	}
	if info.Running {
		t.Fatal("Status().Running should be false")
	}
	if info.State != "stopped" {
		t.Fatalf("Status().State = %q", info.State)
	}
}

func TestStatusPropagatesServiceStatusError(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	fake := &fakeManagedService{statusErr: errors.New("boom")}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}

	if _, err := Status(); err == nil {
		t.Fatal("Status() should propagate service status errors")
	}
}

func TestStatusPropagatesServiceCreateError(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return nil, errors.New("boom")
	}

	if _, err := Status(); err == nil {
		t.Fatal("Status() should propagate service creation errors")
	}
}

func TestUninstallStopsServiceAndRemovesState(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	workspace := t.TempDir()
	if err := WriteMarker(workspace); err != nil {
		t.Fatalf("WriteMarker() error = %v", err)
	}
	if err := writeInstallRecord(Spec{WorkspaceRoot: workspace}); err != nil {
		t.Fatalf("writeInstallRecord() error = %v", err)
	}

	fake := &fakeManagedService{status: kservice.StatusRunning}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}
	if err := Uninstall(); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if !fake.stopCalled {
		t.Fatal("Uninstall() should stop a running service")
	}
	if !fake.uninstallCalled {
		t.Fatal("Uninstall() should uninstall the managed service")
	}
	managed, err := WorkspaceManaged(workspace)
	if err != nil {
		t.Fatalf("WorkspaceManaged() error = %v", err)
	}
	if managed {
		t.Fatal("workspace should not be managed after uninstall")
	}
	if _, err := readInstallRecord(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("readInstallRecord() error = %v, want os.ErrNotExist", err)
	}
}

func TestUninstallRemovesStoppedSystemService(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	workspace := t.TempDir()
	if err := WriteMarker(workspace); err != nil {
		t.Fatalf("WriteMarker() error = %v", err)
	}
	if err := writeInstallRecord(Spec{WorkspaceRoot: workspace}); err != nil {
		t.Fatalf("writeInstallRecord() error = %v", err)
	}
	fake := &fakeManagedService{status: kservice.StatusStopped}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}

	if err := Uninstall(); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if fake.stopCalled {
		t.Fatal("Uninstall() should not stop an already stopped service")
	}
	if !fake.uninstallCalled {
		t.Fatal("Uninstall() should uninstall a stopped service")
	}
}

func TestUninstallRejectsMissingSystemService(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	workspace := t.TempDir()
	if err := writeInstallRecord(Spec{WorkspaceRoot: workspace}); err != nil {
		t.Fatalf("writeInstallRecord() error = %v", err)
	}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return &fakeManagedService{statusErr: kservice.ErrNotInstalled}, nil
	}

	if err := Uninstall(); err == nil {
		t.Fatal("Uninstall() should reject when system service is not installed")
	}
}

func TestUninstallPropagatesSystemStopError(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	workspace := t.TempDir()
	if err := writeInstallRecord(Spec{WorkspaceRoot: workspace}); err != nil {
		t.Fatalf("writeInstallRecord() error = %v", err)
	}
	fake := &fakeManagedService{status: kservice.StatusRunning, stopErr: errors.New("boom")}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}

	if err := Uninstall(); err == nil {
		t.Fatal("Uninstall() should propagate system service stop errors")
	}
}

func TestUninstallPropagatesSystemUninstallError(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	workspace := t.TempDir()
	if err := writeInstallRecord(Spec{WorkspaceRoot: workspace}); err != nil {
		t.Fatalf("writeInstallRecord() error = %v", err)
	}
	fake := &fakeManagedService{status: kservice.StatusStopped, uninstallErr: errors.New("boom")}
	newSystemService = func(spec Spec, program kservice.Interface) (managedService, error) {
		return fake, nil
	}

	if err := Uninstall(); err == nil {
		t.Fatal("Uninstall() should propagate system service uninstall errors")
	}
}

func TestWorkspaceManagedMarkerLifecycle(t *testing.T) {
	workspace := t.TempDir()
	managed, err := WorkspaceManaged(workspace)
	if err != nil {
		t.Fatalf("WorkspaceManaged(initial) error = %v", err)
	}
	if managed {
		t.Fatal("workspace should not be managed before marker write")
	}

	if err := WriteMarker(workspace); err != nil {
		t.Fatalf("WriteMarker error = %v", err)
	}
	managed, err = WorkspaceManaged(workspace)
	if err != nil {
		t.Fatalf("WorkspaceManaged(after write) error = %v", err)
	}
	if !managed {
		t.Fatal("workspace should be managed after marker write")
	}

	if err := RemoveMarker(workspace); err != nil {
		t.Fatalf("RemoveMarker error = %v", err)
	}
	managed, err = WorkspaceManaged(workspace)
	if err != nil {
		t.Fatalf("WorkspaceManaged(after remove) error = %v", err)
	}
	if managed {
		t.Fatal("workspace should not be managed after marker removal")
	}
}

func TestReadInstallRecordRejectsInvalidRecords(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	path, err := installRecordPath()
	if err != nil {
		t.Fatalf("installRecordPath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := readInstallRecord(); err == nil {
		t.Fatal("readInstallRecord() should reject malformed JSON")
	}
	if err := os.WriteFile(path, []byte(`{"workspace":""}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := readInstallRecord(); err == nil {
		t.Fatal("readInstallRecord() should reject a missing workspace")
	}
}

func TestRemoveInstallRecordIgnoresMissingFile(t *testing.T) {
	restore := stubPaths(t)
	defer restore()

	if err := removeInstallRecord(); err != nil {
		t.Fatalf("removeInstallRecord() error = %v", err)
	}
}

func TestServiceStateStringUnknown(t *testing.T) {
	if got := serviceStateString(kservice.StatusUnknown); got != "unknown" {
		t.Fatalf("serviceStateString(StatusUnknown) = %q", got)
	}
}

func TestWorkspaceManagedRejectsInvalidMarker(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(markerPath(workspace), []byte("{"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := WorkspaceManaged(workspace); err == nil {
		t.Fatal("WorkspaceManaged() should reject malformed marker JSON")
	}
}

type fakeManagedService struct {
	status          kservice.Status
	statusErr       error
	startErr        error
	stopErr         error
	restartErr      error
	installErr      error
	uninstallErr    error
	startCalled     bool
	installCalled   bool
	restartCalled   bool
	stopCalled      bool
	uninstallCalled bool
}

func (f *fakeManagedService) Run() error { return nil }

func (f *fakeManagedService) Start() error {
	f.startCalled = true
	return f.startErr
}

func (f *fakeManagedService) Stop() error {
	f.stopCalled = true
	return f.stopErr
}

func (f *fakeManagedService) Restart() error {
	f.restartCalled = true
	return f.restartErr
}

func (f *fakeManagedService) Install() error {
	f.installCalled = true
	return f.installErr
}

func (f *fakeManagedService) Uninstall() error {
	f.uninstallCalled = true
	return f.uninstallErr
}

func (f *fakeManagedService) Status() (kservice.Status, error) {
	return f.status, f.statusErr
}

func stubPaths(t *testing.T) func() {
	t.Helper()
	oldUserConfigDir := userConfigDir
	oldExecutablePath := executablePath
	oldNewSystemService := newSystemService
	configDir := t.TempDir()
	userConfigDir = func() (string, error) { return configDir, nil }
	executablePath = func() (string, error) { return "/usr/local/bin/gizclaw", nil }
	return func() {
		userConfigDir = oldUserConfigDir
		executablePath = oldExecutablePath
		newSystemService = oldNewSystemService
	}
}

func contains(values []string, want string) bool {
	return slices.Contains(values, want)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
