package providers

import (
	"context"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/rafi/gits/internal/logging"
)

const (
	// maxAttempts bounds how many times one throttled request is sent.
	maxAttempts = 3
	// maxRetryAfter caps a server's Retry-After, so one answer cannot stall
	// discovery for long.
	maxRetryAfter = 60 * time.Second
	// defaultRetryAfter is the wait when a throttled response names none.
	defaultRetryAfter = time.Second
)

// hostLimiters is shared by every provider in the process, so projects on
// one host draw from one budget.
var hostLimiters = &limiterRegistry{byHost: map[string]*rate.Limiter{}}

// limiterRegistry holds one request-rate limiter per host.
type limiterRegistry struct {
	mu     sync.Mutex
	byHost map[string]*rate.Limiter
}

// limiter returns host's limiter, lowering it to perSecond when that is
// slower than what it already allows. A non-positive perSecond is unlimited
// and returns nil, leaving the host's limiter as it was.
func (r *limiterRegistry) limiter(host string, perSecond float64) *rate.Limiter {
	if perSecond <= 0 {
		return nil
	}
	limit := rate.Limit(perSecond)
	burst := max(1, int(math.Ceil(perSecond)))

	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.byHost[host]
	if !ok {
		l = rate.NewLimiter(limit, burst)
		r.byHost[host] = l
	} else if limit < l.Limit() {
		l.SetLimit(limit)
		l.SetBurst(burst)
	}
	return l
}

// limitedTransport paces requests per host and, for a retry-safe provider,
// sends a request throttled with 429 or 503 again after the server's
// Retry-After.
type limitedTransport struct {
	base      http.RoundTripper
	limiters  *limiterRegistry
	rate      float64
	retrySafe bool
	log       *slog.Logger

	// sleep waits d unless ctx ends first; tests replace it.
	sleep func(ctx context.Context, d time.Duration) error
	// now reads the clock for an HTTP-date Retry-After; tests replace it.
	now func() time.Time
}

func newLimitedTransport(base http.RoundTripper, perSecond float64, retrySafe bool, log *slog.Logger) *limitedTransport {
	return &limitedTransport{
		base:      base,
		limiters:  hostLimiters,
		rate:      perSecond,
		retrySafe: retrySafe,
		log:       logging.Or(log),
		sleep:     sleepContext,
		now:       time.Now,
	}
}

// RoundTrip waits on the host's limiter before every attempt. The waits end
// with the request's context, which also carries the [http.Client] timeout, so
// an SDK that sends [context.Background] still cannot hang here.
func (t *limitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	limiter := t.limiters.limiter(req.URL.Host, t.rate)
	// A body that cannot be replayed is sent once.
	replayable := req.Body == nil || req.Body == http.NoBody || req.GetBody != nil
	attempts := 1
	if t.retrySafe && replayable {
		attempts = maxAttempts
	}

	for attempt := 1; ; attempt++ {
		if limiter != nil {
			if err := limiter.Wait(ctx); err != nil {
				closeBody(req)
				return nil, err
			}
		}
		sent := req
		if attempt > 1 {
			var err error
			if sent, err = rewind(req); err != nil {
				return nil, err
			}
		}
		resp, err := t.base.RoundTrip(sent)
		if err != nil || attempt == attempts || !throttled(resp.StatusCode) {
			return resp, err
		}

		wait := retryAfter(resp.Header.Get("Retry-After"), t.now())
		t.log.DebugContext(ctx, "retrying throttled request",
			"host", req.URL.Host, "status", resp.StatusCode,
			"attempt", attempt, "wait", wait)
		// Drained so the connection is reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if err := t.sleep(ctx, wait); err != nil {
			return nil, err
		}
	}
}

// throttled reports whether a status asks the client to come back later.
func throttled(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable
}

// rewind returns a copy of req with a fresh body, for sending it again.
func rewind(req *http.Request) (*http.Request, error) {
	sent := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		sent.Body = body
	}
	return sent, nil
}

// closeBody closes a request body a RoundTripper did not get to send, as
// its contract requires.
func closeBody(req *http.Request) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
}

// retryAfter reads a Retry-After header in either form, seconds or an HTTP
// date, capped at maxRetryAfter. Absent or unreadable, it is
// defaultRetryAfter.
func retryAfter(header string, now time.Time) time.Duration {
	header = strings.TrimSpace(header)
	wait := defaultRetryAfter
	if secs, err := strconv.Atoi(header); err == nil {
		wait = time.Duration(secs) * time.Second
	} else if at, err := http.ParseTime(header); err == nil {
		wait = at.Sub(now)
	}
	return min(max(wait, 0), maxRetryAfter)
}

// sleepContext waits d, returning early with ctx's error when it ends first.
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
