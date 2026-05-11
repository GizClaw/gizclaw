package pingstory_test

import (
	"strings"
	"sync"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
)

func TestPingUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "000-ping")
	h.StartServerFromFixture("server_config.yaml")

	h.CreateContext("client-a").MustSucceed(t)
	h.CreateContext("client-b").MustSucceed(t)
	h.WaitForPing("client-a")
	h.WaitForPing("client-b")

	t.Run("single client can ping repeatedly", func(t *testing.T) {
		for range 3 {
			result, err := h.RunCLIUntilSuccess("ping", "--context", "client-a")
			if err != nil {
				t.Fatal(err)
			}
			assertPingOutput(t, result.Stdout)
		}
	})

	t.Run("multiple clients can ping concurrently", func(t *testing.T) {
		contexts := []string{"client-a", "client-b"}
		type outcome struct {
			result clitest.Result
			err    error
		}
		results := make(chan outcome, len(contexts))

		var wg sync.WaitGroup
		for _, ctxName := range contexts {
			ctxName := ctxName
			wg.Add(1)
			go func() {
				defer wg.Done()
				result, err := h.RunCLIUntilSuccess("ping", "--context", ctxName)
				results <- outcome{result: result, err: err}
			}()
		}

		wg.Wait()
		close(results)

		for outcome := range results {
			if outcome.err != nil {
				t.Fatal(outcome.err)
			}
			assertPingOutput(t, outcome.result.Stdout)
		}
	})
}

func assertPingOutput(t *testing.T, stdout string) {
	t.Helper()

	for _, fragment := range []string{"Server Time:", "RTT:", "Clock Diff:"} {
		if !strings.Contains(stdout, fragment) {
			t.Fatalf("ping output missing %q:\n%s", fragment, stdout)
		}
	}
}
