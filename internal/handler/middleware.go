package handler

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ipRateLimiter holds per-IP token bucket limiters.
type ipRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*entry
	rpm      int
}

type entry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newIPRateLimiter(rpm int) *ipRateLimiter {
	rl := &ipRateLimiter{
		limiters: make(map[string]*entry),
		rpm:      rpm,
	}
	go rl.cleanup()
	return rl
}

func (rl *ipRateLimiter) get(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	e, ok := rl.limiters[ip]
	if !ok {
		// Allow rpm requests per minute with a burst of rpm/4 (min 1).
		burst := rl.rpm / 4
		if burst < 1 {
			burst = 1
		}
		lim := rate.NewLimiter(rate.Every(time.Minute/time.Duration(rl.rpm)), burst)
		e = &entry{limiter: lim}
		rl.limiters[ip] = e
	}
	e.lastSeen = time.Now()
	return e.limiter
}

// cleanup removes stale entries every 5 minutes.
func (rl *ipRateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		rl.mu.Lock()
		for ip, e := range rl.limiters {
			if time.Since(e.lastSeen) > 10*time.Minute {
				delete(rl.limiters, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// RateLimit returns a middleware that limits requests per IP.
// rpm <= 0 disables rate limiting.
func RateLimit(rpm int) func(http.Handler) http.Handler {
	if rpm <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	rl := newIPRateLimiter(rpm)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}
			if !rl.get(ip).Allow() {
				http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
