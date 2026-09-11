package giznet

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestWriteBudgetRejectsOversizeReservations(t *testing.T) {
	budget := NewWriteBudget(8)
	if acquired, _, err := budget.TryAcquire(9); acquired || !errors.Is(err, ErrPacketTooLarge) {
		t.Fatalf("TryAcquire(9) = %t, %v", acquired, err)
	}
	var none *WriteBudget
	if acquired, _, err := none.TryAcquire(1 << 40); !acquired || err != nil {
		t.Fatalf("nil budget TryAcquire = %t, %v", acquired, err)
	}
	if acquired, _, err := budget.TryAcquire(0); !acquired || err != nil || budget.Used() != 0 {
		t.Fatalf("zero TryAcquire = %t, %v, used=%d", acquired, err, budget.Used())
	}
}

func TestWriteBudgetWakesWaitersOnRelease(t *testing.T) {
	budget := NewWriteBudget(8)
	if acquired, _, err := budget.TryAcquire(6); !acquired || err != nil {
		t.Fatalf("first TryAcquire = %t, %v", acquired, err)
	}
	acquired, wake, err := budget.TryAcquire(4)
	if acquired || err != nil || wake == nil {
		t.Fatalf("over-limit TryAcquire = %t, %v, wake=%v", acquired, err, wake)
	}
	select {
	case <-wake:
		t.Fatal("wake fired before release")
	default:
	}
	budget.Release(6)
	select {
	case <-wake:
	case <-time.After(time.Second):
		t.Fatal("release did not wake waiter")
	}
	if acquired, _, err := budget.TryAcquire(4); !acquired || err != nil || budget.Used() != 4 {
		t.Fatalf("retry TryAcquire = %t, %v, used=%d", acquired, err, budget.Used())
	}
	budget.Release(100)
	if got := budget.Used(); got != 0 {
		t.Fatalf("used after over-release = %d", got)
	}
}

func TestWriteBudgetConcurrentAccounting(t *testing.T) {
	budget := NewWriteBudget(16)
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for range 100 {
				for {
					acquired, wake, err := budget.TryAcquire(4)
					if err != nil {
						t.Error(err)
						return
					}
					if acquired {
						break
					}
					<-wake
				}
				if used := budget.Used(); used > budget.Limit() {
					t.Errorf("used %d exceeds limit %d", used, budget.Limit())
				}
				budget.Release(4)
			}
		})
	}
	wg.Wait()
	if got := budget.Used(); got != 0 {
		t.Fatalf("used after concurrent release = %d", got)
	}
}
