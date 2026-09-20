package finance

import (
	"sync"
	"time"
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type FrozenClock struct {
	mu   sync.Mutex
	now  time.Time
}

func NewFrozenClock(t time.Time) *FrozenClock {
	return &FrozenClock{now: t.UTC()}
}

func (c *FrozenClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *FrozenClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
