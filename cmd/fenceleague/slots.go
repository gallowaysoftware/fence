package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// slotPoll is how often a request waiting for an endpoint slot tries again.
const slotPoll = 100 * time.Millisecond

// Slots caps an endpoint's in-flight requests across every fenceleague process on this
// machine: a slot is an flock on one of n files, so two leagues sharing a node share its cap.
type Slots struct {
	dir string
	n   int
}

func NewSlots(root, endpoint string, n int) (*Slots, error) {
	dir := filepath.Join(root, endpoint)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Slots{dir: dir, n: max(1, n)}, nil
}

// Acquire blocks until a slot is free or ctx ends; release frees it.
func (s *Slots) Acquire(ctx context.Context) (release func(), err error) {
	for {
		for i := range s.n {
			f, err := os.OpenFile(filepath.Join(s.dir, fmt.Sprintf("%d.lock", i)), os.O_CREATE|os.O_RDWR, 0o644)
			if err != nil {
				return nil, err
			}
			if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil {
				return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
			}
			f.Close()
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(slotPoll):
		}
	}
}
