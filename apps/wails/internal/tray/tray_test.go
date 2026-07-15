package tray

import "testing"

type fakeBackend struct {
	started []Pod
	updated []Pod
	stopped bool
}

func (f *fakeBackend) Start(pods []Pod)  { f.started = pods }
func (f *fakeBackend) Update(pods []Pod) { f.updated = pods }
func (f *fakeBackend) Stop()             { f.stopped = true }

func TestManagerProjectsPodNavigationWithoutSharingMutableState(t *testing.T) {
	backend := &fakeBackend{}
	manager := &Manager{backend: backend}
	pods := []Pod{{ID: "local", Label: "Local Lab", Section: "Local"}, {ID: "remote", Label: "Remote Lab · 24 Servers", Section: "Remote"}}
	manager.Start(pods)
	pods[0].Label = "mutated"
	if len(backend.started) != 2 || backend.started[0].Label != "Local Lab" || backend.started[0].Section != "Local" {
		t.Fatalf("Start projection = %+v", backend.started)
	}
	manager.Update(pods[1:])
	manager.Stop()
	if len(backend.updated) != 1 || backend.updated[0].ID != "remote" || !backend.stopped {
		t.Fatalf("Update/Stop = %+v/%v", backend.updated, backend.stopped)
	}
}
