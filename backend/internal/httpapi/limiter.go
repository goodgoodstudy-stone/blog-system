package httpapi

import (
	"sync"
	"time"
)

type attempt struct {
	count int
	until time.Time
}
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]attempt
}

func newLoginLimiter() *loginLimiter { return &loginLimiter{attempts: map[string]attempt{}} }
func (l *loginLimiter) blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	v := l.attempts[key]
	if time.Now().After(v.until) {
		delete(l.attempts, key)
		return false
	}
	return v.count >= 5
}
func (l *loginLimiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	v := l.attempts[key]
	if now.After(v.until) {
		v = attempt{until: now.Add(15 * time.Minute)}
	}
	v.count++
	l.attempts[key] = v
	if len(l.attempts) > 1000 {
		for k, a := range l.attempts {
			if now.After(a.until) {
				delete(l.attempts, k)
			}
		}
	}
}
func (l *loginLimiter) reset(key string) { l.mu.Lock(); delete(l.attempts, key); l.mu.Unlock() }
