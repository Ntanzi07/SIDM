package users

import (
	"sync"
	"time"
)

// limiter segura a adivinhação de senha: depois de maxFailures erros
// seguidos num usuário, o login dele fica bloqueado por lockout. Fica na
// memória do servidor (reiniciar zera), o que basta numa rede interna.
type limiter struct {
	mu       sync.Mutex
	failures map[string]*attempts
	now      func() time.Time
}

type attempts struct {
	count int
	until time.Time // bloqueado até
}

const (
	maxFailures = 5
	lockout     = 5 * time.Minute
)

func newLimiter(now func() time.Time) *limiter {
	return &limiter{failures: map[string]*attempts{}, now: now}
}

func (l *limiter) allow(user string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.failures[user]
	return !ok || !l.now().Before(a.until)
}

func (l *limiter) fail(user string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.failures[user]
	if !ok {
		a = &attempts{}
		l.failures[user] = a
	}
	a.count++
	if a.count >= maxFailures {
		a.count, a.until = 0, l.now().Add(lockout)
	}
}

func (l *limiter) reset(user string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, user)
}
