package api

import (
	"fmt"
	"net"
	"net/http"
	"time"

	rs "github.com/altlimit/restruct"

	dscache "github.com/altlimit/dsorm/cache"
)

// rlCache is a package-level in-memory cache used for rate limiting.
var Cache dscache.Cache

// RateLimitByIP extracts the client IP from r, then checks the sliding-window
// rate limit via dsorm cache. Returns a non-nil rs.Error (HTTP 429) when the
// caller has exceeded the limit for the given endpoint within the window.
func RateLimitByIP(r *http.Request, endpoint string, limit int, window time.Duration) error {
	ip := clientIP(r)
	key := fmt.Sprintf("rl:%s:%s", endpoint, ip)

	result, err := Cache.RateLimit(r.Context(), key, limit, window)
	if err != nil {
		// On cache errors, fail open — don't block requests.
		return nil
	}
	if !result.Allowed {
		return rs.Error{
			Status:  http.StatusTooManyRequests,
			Message: "rate limit exceeded, try again later",
		}
	}
	return nil
}

// clientIP returns the best-effort client IP address, respecting common
// reverse-proxy headers (X-Forwarded-For, X-Real-Ip) before falling back
// to r.RemoteAddr.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For may be a comma-separated list; use the first entry.
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}
	if xri := r.Header.Get("X-Real-Ip"); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
