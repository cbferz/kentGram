package limiter

import (
	"sync"
	"testing"
)

func TestPerKeyAndGlobalLimits(t *testing.T) {
	c := New(3, 2)

	r1, ok := c.TryAcquire(1)
	if !ok {
		t.Fatal("first permit must be granted")
	}
	if _, ok := c.TryAcquire(1); !ok {
		t.Fatal("second permit for the same user must be granted")
	}
	if _, ok := c.TryAcquire(1); ok {
		t.Fatal("a third concurrent permit for one user must be refused")
	}
	if _, ok := c.TryAcquire(2); !ok {
		t.Fatal("another user must not be affected by user 1's limit")
	}
	if _, ok := c.TryAcquire(3); ok {
		t.Fatal("the global limit of 3 must refuse a fourth permit")
	}

	r1()
	if _, ok := c.TryAcquire(3); !ok {
		t.Fatal("releasing a permit must free capacity")
	}
}

func TestReleaseIsIdempotent(t *testing.T) {
	c := New(5, 5)
	release, _ := c.TryAcquire(1)
	release()
	release()
	release()
	if c.InUse() != 0 {
		t.Fatalf("InUse = %d, want 0; double release must not go negative", c.InUse())
	}
	if _, ok := c.TryAcquire(1); !ok {
		t.Fatal("permit should be available after release")
	}
}

func TestConcurrentUseNeverExceedsTheLimit(t *testing.T) {
	const limit = 4
	c := New(limit, limit)
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		peak     int
		inFlight int
	)
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			release, ok := c.TryAcquire(id % 3)
			if !ok {
				return
			}
			mu.Lock()
			inFlight++
			if inFlight > peak {
				peak = inFlight
			}
			mu.Unlock()

			mu.Lock()
			inFlight--
			mu.Unlock()
			release()
		}(int64(i))
	}
	wg.Wait()
	if peak > limit {
		t.Fatalf("peak concurrency %d exceeded the limit %d", peak, limit)
	}
	if c.InUse() != 0 {
		t.Fatalf("permits leaked: %d still held", c.InUse())
	}
}
