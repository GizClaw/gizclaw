//go:build darwin

package tray

/*
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>
void gizclawTrayStart(void);
void gizclawTrayClear(void);
void gizclawTrayAddSection(const char *label);
void gizclawTrayAddPod(const char *podID, const char *label);
void gizclawTrayFinish(const char *openWindowLabel, const char *quitLabel);
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
	C.gizclawTrayClear()
	sectionName := ""
	for _, pod := range pods {
		if pod.Section != sectionName {
			sectionName = pod.Section
			section := C.CString(sectionName)
			C.gizclawTrayAddSection(section)
			C.free(unsafe.Pointer(section))
		}
		id := C.CString(pod.ID)
		label := C.CString(pod.Label)
		C.gizclawTrayAddPod(id, label)
		C.free(unsafe.Pointer(id))
		C.free(unsafe.Pointer(label))
	}
	openWindow := C.CString(b.labels.OpenWindow)
	quit := C.CString(b.labels.Quit)
	C.gizclawTrayFinish(openWindow, quit)
	C.free(unsafe.Pointer(openWindow))
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
		go b.callbacks.OpenWindow()
	}
}

//export gizclawGoTrayOpenPod
func gizclawGoTrayOpenPod(podID *C.char) {
	id := C.GoString(podID)
	darwinMu.RLock()
	b := darwinCurrent
	darwinMu.RUnlock()
	if b != nil && b.callbacks.OpenPod != nil {
		go b.callbacks.OpenPod(id)
	}
}

//export gizclawGoTrayQuit
func gizclawGoTrayQuit() {
	darwinMu.RLock()
	b := darwinCurrent
	darwinMu.RUnlock()
	if b != nil && b.callbacks.Quit != nil {
		go b.callbacks.Quit()
	}
}
