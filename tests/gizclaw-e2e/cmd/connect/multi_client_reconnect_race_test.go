//go:build gizclaw_e2e

package connect_test

import (
	"sync"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestMultiClientReconnectRaceUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "403-multi-client-reconnect-race")
	h.StartServerFromFixture("server_config.yaml")

	contexts := []string{"alpha", "beta"}
	for _, name := range contexts {
		h.CreateContext(name).MustSucceed(t)
		if _, err := h.RunCLIUntilSuccess("connect", "ping", "--context", name); err != nil {
			t.Fatal(err)
		}
	}

	h.StopServer()
	h.RestartServer()

	var wg sync.WaitGroup
	errs := make(chan error, len(contexts))
	for _, name := range contexts {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			_, err := h.RunCLIUntilSuccess("connect", "ping", "--context", name)
			errs <- err
		}(name)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}
