//go:build !darwin

package tray

import (
	"encoding/base64"
	"sync"

	"github.com/getlantern/systray"
)

type genericBackend struct {
	callbacks  Callbacks
	labels     Labels
	once       sync.Once
	mu         sync.Mutex
	pods       []Pod
	items      map[string]*systray.MenuItem
	itemLabels map[string]string
	readyDone  bool
}

func newPlatformBackend(callbacks Callbacks, labels Labels) platformBackend {
	return &genericBackend{callbacks: callbacks, labels: labels, items: map[string]*systray.MenuItem{}, itemLabels: map[string]string{}}
}

func (b *genericBackend) Start(pods []Pod) {
	b.mu.Lock()
	b.pods = pods
	b.mu.Unlock()
	b.once.Do(func() { go systray.Run(b.ready, func() {}) })
}

func (b *genericBackend) Update(pods []Pod) {
	b.mu.Lock()
	b.pods = pods
	ready := b.readyDone
	b.mu.Unlock()
	if ready {
		b.syncItems(pods)
	}
}

func (b *genericBackend) Stop() { systray.Quit() }

func (b *genericBackend) ready() {
	icon, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAQAAAC1+jfqAAAAWElEQVR42mNgGAWjYBSMglEwCkbB////Gf4zMDAw/GdgYGBg+M/AwMDA8J+BgYHhPwMDA8N/BgYGBv4zMDAw/GdgYGBg+M/AwMDA8J+BgYHhPwMDA8N/BoYBAF0fFQHL4YBDAAAAAElFTkSuQmCC")
	systray.SetIcon(icon)
	systray.SetTooltip("GizClaw")
	open := systray.AddMenuItem(b.labels.OpenWindow, b.labels.OpenWindow)
	go func() {
		for range open.ClickedCh {
			if b.callbacks.OpenWindow != nil {
				b.callbacks.OpenWindow()
			}
		}
	}()
	systray.AddSeparator()
	b.mu.Lock()
	pods := append([]Pod(nil), b.pods...)
	b.readyDone = true
	b.mu.Unlock()
	b.syncItems(pods)
	systray.AddSeparator()
	quit := systray.AddMenuItem(b.labels.Quit, b.labels.Quit)
	go func() {
		<-quit.ClickedCh
		if b.callbacks.Quit != nil {
			b.callbacks.Quit()
		}
	}()
}

func (b *genericBackend) syncItems(pods []Pod) {
	b.mu.Lock()
	defer b.mu.Unlock()
	seen := map[string]bool{}
	for _, pod := range pods {
		seen[pod.ID] = true
		if item := b.items[pod.ID]; item != nil {
			if b.itemLabels[pod.ID] != pod.Label {
				item.SetTitle(pod.Label)
				b.itemLabels[pod.ID] = pod.Label
			}
			item.Show()
			continue
		}
		pod := pod
		if len(b.items) > 0 {
			systray.AddSeparator()
		}
		parent := systray.AddMenuItem(pod.Label, pod.Label)
		item := parent.AddSubMenuItem(b.labels.OpenPod, b.labels.OpenPod)
		b.items[pod.ID] = parent
		b.itemLabels[pod.ID] = pod.Label
		go func() {
			for range item.ClickedCh {
				if b.callbacks.OpenPod != nil {
					b.callbacks.OpenPod(pod.ID)
				}
			}
		}()
	}
	for id, item := range b.items {
		if !seen[id] {
			item.Hide()
		}
	}
}
