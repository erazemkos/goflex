package auth

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/goflex/goflex/pkg/httperr"
)

type Credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type LoginVerifier func(Credentials) (string, bool)

func (a *Auth) LoginHandler(verify LoginVerifier) gin.HandlerFunc {
	limiter := newLimiter(10, time.Minute)
	return func(c *gin.Context) {
		key := c.ClientIP()
		if !limiter.Allow(key) {
			httperr.Write(c, http.StatusTooManyRequests, httperr.New("rate_limited", "too many attempts", nil))
			return
		}
		var cr Credentials
		if err := c.ShouldBindJSON(&cr); err != nil {
			limiter.RecordFailure(key)
			httperr.Write(c, http.StatusBadRequest, httperr.New("bad_request", "invalid credentials payload", nil))
			return
		}
		if id, ok := verify(cr); ok {
			limiter.Reset(key)
			a.Login(c, id)
			c.JSON(http.StatusOK, gin.H{"id": id})
			return
		}
		limiter.RecordFailure(key)
		httperr.Write(c, http.StatusUnauthorized, httperr.New("invalid_credentials", "invalid credentials", nil))
	}
}

func (a *Auth) LogoutHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		a.Logout(c)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

type limiter struct {
	mu  sync.Mutex
	n   int
	win time.Duration
	now func() time.Time
	m   map[string][]time.Time
}

var limiterNow = time.Now

func newLimiter(n int, win time.Duration) *limiter {
	return &limiter{n: n, win: win, now: limiterNow, m: map[string][]time.Time{}}
}
func (l *limiter) Allow(k string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pruneLocked(k)
	return len(l.m[k]) < l.n
}
func (l *limiter) RecordFailure(k string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pruneLocked(k)
	l.m[k] = append(l.m[k], l.now())
}
func (l *limiter) Reset(k string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.m, k)
}
func (l *limiter) pruneLocked(k string) {
	now := l.now()
	xs := l.m[k][:0]
	for _, t := range l.m[k] {
		if now.Sub(t) < l.win {
			xs = append(xs, t)
		}
	}
	l.m[k] = xs
}
