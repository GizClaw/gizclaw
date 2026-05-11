package pingmultiround_test

import (
	"strings"
	"sync"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
)

func TestPingMultiRoundUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "002-ping-multi-round")
	h.StartServerFromFixture("server_config.yaml")

	contexts := []string{"client-a", "client-b", "client-c"}
	for _, name := range contexts {
		h.CreateContext(name).MustSucceed(t)
		h.WaitForPing(name)
	}

	for round := range 3 {
		t.Run("sequential-round-"+itoa(round), func(t *testing.T) {
			for _, name := range contexts {
				result, err := h.RunCLIUntilSuccess("ping", "--context", name)
				if err != nil {
					t.Fatal(err)
				}
				assertPingOutput(t, result.Stdout)
			}
		})

		t.Run("concurrent-round-"+itoa(round), func(t *testing.T) {
			runConcurrentPings(t, h, []string{"client-a", "client-b"})
		})
	}

	finalCheck, err := h.RunCLIUntilSuccess("ping", "--context", "client-c")
	if err != nil {
		t.Fatal(err)
	}
	assertPingOutput(t, finalCheck.Stdout)
}

func runConcurrentPings(t *testing.T, h *clitest.Harness, contexts []string) {
	t.Helper()

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
}

func assertPingOutput(t *testing.T, stdout string) {
	t.Helper()

	for _, fragment := range []string{"Server Time:", "RTT:", "Clock Diff:"} {
		if !strings.Contains(stdout, fragment) {
			t.Fatalf("ping output missing %q:\n%s", fragment, stdout)
		}
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
