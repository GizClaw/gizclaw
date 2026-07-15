//go:build darwin

package tray

/*
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>
void gizclawTrayStart(void);
void gizclawTrayClear(const char *openWindowLabel);
void gizclawTrayAddPod(const char *podID, const char *label, const char *openPodLabel);
void gizclawTrayFinish(const char *quitLabel);
void gizclawTrayStop(void);
*/
import "C"

import (
	"sync"
	"unsafe"
)

type darwinBackend struct {
	callbacks Callbacks
	labels    Labels
	started   bool
}

var (
	darwinMu      sync.RWMutex
	darwinCurrent *darwinBackend
)

func newPlatformBackend(callbacks Callbacks, labels Labels) platformBackend {
	return &darwinBackend{callbacks: callbacks, labels: labels}
}

func (b *darwinBackend) Start(pods []Pod) {
	darwinMu.Lock()
	darwinCurrent = b
	darwinMu.Unlock()
	if !b.started {
		C.gizclawTrayStart()
		b.started = true
	}
	b.Update(pods)
}

func (b *darwinBackend) Update(pods []Pod) {
	if !b.started {
		return
	}
	openWindow := C.CString(b.labels.OpenWindow)
	C.gizclawTrayClear(openWindow)
	C.free(unsafe.Pointer(openWindow))
	for _, pod := range pods {
		id := C.CString(pod.ID)
		label := C.CString(pod.Label)
		openPod := C.CString(b.labels.OpenPod)
		C.gizclawTrayAddPod(id, label, openPod)
		C.free(unsafe.Pointer(id))
		C.free(unsafe.Pointer(label))
		C.free(unsafe.Pointer(openPod))
	}
	quit := C.CString(b.labels.Quit)
	C.gizclawTrayFinish(quit)
	C.free(unsafe.Pointer(quit))
}

func (b *darwinBackend) Stop() {
	if b.started {
		C.gizclawTrayStop()
		b.started = false
	}
	darwinMu.Lock()
	if darwinCurrent == b {
		darwinCurrent = nil
	}
	darwinMu.Unlock()
}

//export gizclawGoTrayOpenWindow
func gizclawGoTrayOpenWindow() {
	darwinMu.RLock()
	b := darwinCurrent
	darwinMu.RUnlock()
	if b != nil && b.callbacks.OpenWindow != nil {
		b.callbacks.OpenWindow()
	}
}

//export gizclawGoTrayOpenPod
func gizclawGoTrayOpenPod(podID *C.char) {
	darwinMu.RLock()
	b := darwinCurrent
	darwinMu.RUnlock()
	if b != nil && b.callbacks.OpenPod != nil {
		b.callbacks.OpenPod(C.GoString(podID))
	}
}

//export gizclawGoTrayQuit
func gizclawGoTrayQuit() {
	darwinMu.RLock()
	b := darwinCurrent
	darwinMu.RUnlock()
	if b != nil && b.callbacks.Quit != nil {
		b.callbacks.Quit()
	}
}
