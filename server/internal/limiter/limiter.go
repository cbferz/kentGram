// Package limiter bounds how many expensive operations (uploads, image
// processing) run at once, both overall and per user, so that a single client
// cannot exhaust memory or disk bandwidth. It depends only on the standard
// library.
package limiter

import "sync"

// Concurrency hands out non-blocking permits. A caller that cannot get one
// should answer 429 instead of queueing, which keeps latency predictable.
type Concurrency struct {
	mu        sync.Mutex
	global    int
	perKey    int
	usedTotal int
	used      map[int64]int
}

func New(global, perKey int) *Concurrency {
	return &Concurrency{global: global, perKey: perKey, used: make(map[int64]int)}
}

// TryAcquire returns a release function and true when a permit is available
// for key. The release function is idempotent.
func (c *Concurrency) TryAcquire(key int64) (release func(), ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.usedTotal >= c.global || c.used[key] >= c.perKey {
		return nil, false
	}
	c.usedTotal++
	c.used[key]++

	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.usedTotal--
			if c.used[key]--; c.used[key] <= 0 {
				delete(c.used, key)
			}
		})
	}, true
}

// InUse reports the number of permits currently held.
func (c *Concurrency) InUse() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.usedTotal
}
