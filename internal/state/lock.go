package state

import "sync"

// Locker serializes writes to shared state in command code.
type Locker struct {
	mu sync.Mutex
}

// Lock acquires the lock.
func (l *Locker) Lock() {
	l.mu.Lock()
}

// Unlock releases the lock.
func (l *Locker) Unlock() {
	l.mu.Unlock()
}
