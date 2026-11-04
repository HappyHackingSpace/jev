package jev

import (
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

// RetryPolicy controls retries of rate-limited, overloaded and failed
// requests. The defaults match TypeSafe's official SDKs.
type RetryPolicy struct {
	// MaxRetries is the number of retries after the first attempt; 0
	// disables retries.
	MaxRetries int
	// InitialBackoff is the wait before the first retry; it doubles each
	// time up to MaxBackoff.
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	// Jitter randomizes each wait by up to this fraction, so clients that
	// were throttled together do not retry together.
	Jitter float64
}

// DefaultRetryPolicy returns the policy a new client uses.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxRetries: 2, InitialBackoff: 500 * time.Millisecond, MaxBackoff: 5 * time.Second, Jitter: 0.25}
}

// maxRetryAfter caps a server-requested wait so a bad header cannot stall a
// caller for hours.
const maxRetryAfter = time.Minute

func (p RetryPolicy) backoff(retry int) time.Duration {
	d := float64(p.InitialBackoff) * math.Pow(2, float64(retry))
	d = math.Min(d, float64(p.MaxBackoff))
	d *= 1 + p.Jitter*(2*rand.Float64()-1)
	return time.Duration(math.Max(d, 0))
}

// retryAfter reads the wait the server asked for, preferring the
// millisecond header TypeSafe sends.
func retryAfter(h http.Header) (time.Duration, bool) {
	if v, err := strconv.ParseFloat(h.Get("Retry-After-Ms"), 64); err == nil && v >= 0 {
		return min(time.Duration(v*float64(time.Millisecond)), maxRetryAfter), true
	}
	ra := h.Get("Retry-After")
	if v, err := strconv.ParseFloat(ra, 64); err == nil && v >= 0 {
		return min(time.Duration(v*float64(time.Second)), maxRetryAfter), true
	}
	if t, err := http.ParseTime(ra); err == nil {
		return min(max(time.Until(t), 0), maxRetryAfter), true
	}
	return 0, false
}
