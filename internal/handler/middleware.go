package handler

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	authsvc "github.com/poyrazk/cloudtalk/internal/auth"
	"github.com/poyrazk/cloudtalk/internal/metrics"
	apptrace "github.com/poyrazk/cloudtalk/internal/tracing"
	"golang.org/x/time/rate"
)

// keyedRateLimiter holds token bucket limiters keyed by an arbitrary string.
type keyedRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*entry
	rpm      int
}

type entry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newKeyedRateLimiter(rpm int) *keyedRateLimiter {
	rl := &keyedRateLimiter{
		limiters: make(map[string]*entry),
		rpm:      rpm,
	}
	go rl.cleanup()
	return rl
}

func (rl *keyedRateLimiter) get(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	e, ok := rl.limiters[key]
	if !ok {
		burst := rl.rpm / 4
		if burst < 1 {
			burst = 1
		}
		lim := rate.NewLimiter(rate.Every(time.Minute/time.Duration(rl.rpm)), burst)
		e = &entry{limiter: lim}
		rl.limiters[key] = e
	}
	e.lastSeen = time.Now()
	return e.limiter
}

func (rl *keyedRateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		rl.mu.Lock()
		for key, e := range rl.limiters {
			if time.Since(e.lastSeen) > 10*time.Minute {
				delete(rl.limiters, key)
			}
		}
		rl.mu.Unlock()
	}
}

func userOrIPKey(r *http.Request) (key string, scope string) {
	if userID, ok := authsvc.UserIDFromContext(r.Context()); ok {
		return "user:" + userID.String(), "user"
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	return "ip:" + ip, "ip"
}

// RateLimit returns a middleware that limits requests per IP.
// rpm <= 0 disables rate limiting.
func RateLimit(rpm int) func(http.Handler) http.Handler {
	if rpm <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	rl := newKeyedRateLimiter(rpm)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}
			if !rl.get(ip).Allow() {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				if err := json.NewEncoder(w).Encode(map[string]string{"error": "too many requests"}); err != nil {
					slog.Error("http: write auth throttle response", "err", err)
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func AuthenticatedRateLimit(group string, rpm int) func(http.Handler) http.Handler {
	if rpm <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	rl := newKeyedRateLimiter(rpm)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key, scope := userOrIPKey(r)
			if rl.get(key).Allow() {
				next.ServeHTTP(w, r)
				return
			}

			metrics.HTTPThrottledRequestsTotal.WithLabelValues(group, scope).Inc()
			slog.Warn("http: request throttled",
				"group", group,
				"scope", scope,
				"request_id", chimiddleware.GetReqID(r.Context()),
				"trace_id", apptrace.TraceIDFromContext(r.Context()),
				"path", r.URL.Path,
			)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			if err := json.NewEncoder(w).Encode(map[string]string{"error": "too many requests"}); err != nil {
				slog.Error("http: write throttle response", "err", err)
			}
		})
	}
}
