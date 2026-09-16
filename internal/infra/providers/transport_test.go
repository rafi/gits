package providers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/rafi/gits/domain"
)

// roundTripFunc stubs the network under a limitedTransport.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// respond is a stub server answering each call with the next status in
// statuses, repeating the last, and counting the calls.
func respond(calls *atomic.Int32, header http.Header, statuses ...int) roundTripFunc {
	return func(req *http.Request) (*http.Response, error) {
		n := int(calls.Add(1))
		status := statuses[min(n, len(statuses))-1]
		return &http.Response{
			StatusCode: status,
			Header:     header.Clone(),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}, nil
	}
}

// newTestTransport builds a transport on its own limiter registry, so tests
// neither share nor leave behind a host's limiter, and records its sleeps
// instead of taking them.
func newTestTransport(base http.RoundTripper, perSecond float64, retrySafe bool) (*limitedTransport, *[]time.Duration) {
	t := newLimitedTransport(base, perSecond, retrySafe, nil)
	t.limiters = &limiterRegistry{byHost: map[string]*rate.Limiter{}}
	var slept []time.Duration
	t.sleep = func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}
	return t, &slept
}

func get(ctx context.Context, rt http.RoundTripper) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://forge.test/api", nil)
	if err != nil {
		return nil, err
	}
	resp, err := rt.RoundTrip(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	return resp, err
}

// TestLimitedTransportPaces proves requests beyond the burst wait for the
// host's rate: at 20/s with a burst of 20, five more requests cost at least
// four gaps of 50ms.
func TestLimitedTransportPaces(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	rt, _ := newTestTransport(respond(&calls, nil, http.StatusOK), 20, true)
	start := time.Now()
	for range 25 {
		if _, err := get(t.Context(), rt); err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed < 200*time.Millisecond {
		t.Errorf("25 requests at 20/s took %v, want at least 200ms", elapsed)
	}
}

// TestLimitedTransportZeroIsUnlimited proves rateLimit 0 sends without
// waiting and registers no limiter.
func TestLimitedTransportZeroIsUnlimited(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	rt, _ := newTestTransport(respond(&calls, nil, http.StatusOK), 0, true)
	start := time.Now()
	for range 200 {
		if _, err := get(t.Context(), rt); err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("200 unlimited requests took %v", elapsed)
	}
	if len(rt.limiters.byHost) != 0 {
		t.Errorf("limiters = %v, want none for an unlimited rate", rt.limiters.byHost)
	}
}

// TestLimiterRegistrySharesHosts proves one limiter per host, lowered by a
// slower rate and never raised by a faster or unlimited one.
func TestLimiterRegistrySharesHosts(t *testing.T) {
	t.Parallel()

	r := &limiterRegistry{byHost: map[string]*rate.Limiter{}}
	first := r.limiter("forge.test", 10)
	if got := r.limiter("forge.test", 5); got != first {
		t.Fatal("same host returned a second limiter")
	}
	if first.Limit() != 5 || first.Burst() != 5 {
		t.Errorf("after a slower rate: limit %v burst %d, want 5 and 5", first.Limit(), first.Burst())
	}
	r.limiter("forge.test", 10)
	if r.limiter("forge.test", 0) != nil {
		t.Error("unlimited rate returned a limiter")
	}
	if first.Limit() != 5 {
		t.Errorf("faster and unlimited rates raised the limit to %v", first.Limit())
	}
	if r.limiter("other.test", 10) == first {
		t.Error("another host shares the limiter")
	}
	if l := r.limiter("slow.test", 0.5); l.Burst() != 1 {
		t.Errorf("fractional rate burst = %d, want 1", l.Burst())
	}
}

// TestLimitedTransportRetries proves a throttled request is sent at most
// three times, waiting Retry-After between, and the last response returned;
// and that a request that is not throttled, or a provider that is not
// retry-safe, is sent once.
func TestLimitedTransportRetries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		retrySafe bool
		statuses  []int
		wantCalls int32
		wantSleep int
		want      int
	}{
		{"429 gives up after three", true, []int{429}, 3, 2, 429},
		{"503 recovers", true, []int{503, 200}, 2, 1, 200},
		{"500 is not throttling", true, []int{500}, 1, 0, 500},
		{"not retry-safe sends once", false, []int{429}, 1, 0, 429},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			header := http.Header{"Retry-After": {"2"}}
			rt, slept := newTestTransport(respond(&calls, header, tt.statuses...), 0, tt.retrySafe)
			resp, err := get(t.Context(), rt)
			if err != nil {
				t.Fatalf("RoundTrip: %v", err)
			}
			if resp.StatusCode != tt.want || calls.Load() != tt.wantCalls {
				t.Errorf("status %d after %d calls, want %d after %d",
					resp.StatusCode, calls.Load(), tt.want, tt.wantCalls)
			}
			if len(*slept) != tt.wantSleep {
				t.Fatalf("slept %v, want %d waits", *slept, tt.wantSleep)
			}
			for _, d := range *slept {
				if d != 2*time.Second {
					t.Errorf("waited %v, want the Retry-After of 2s", d)
				}
			}
		})
	}
}

// TestRetryAfter proves both header forms, the cap, and the default.
func TestRetryAfter(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	date := func(d time.Duration) string { return now.Add(d).Format(http.TimeFormat) }
	tests := []struct {
		header string
		want   time.Duration
	}{
		{"", time.Second},
		{"soon", time.Second},
		{"7", 7 * time.Second},
		{" 7 ", 7 * time.Second},
		{"0", 0},
		{"-3", 0},
		{"600", maxRetryAfter},
		{date(30 * time.Second), 30 * time.Second},
		{date(-time.Minute), 0},
		{date(time.Hour), maxRetryAfter},
	}
	for _, tt := range tests {
		if got := retryAfter(tt.header, now); got != tt.want {
			t.Errorf("retryAfter(%q) = %v, want %v", tt.header, got, tt.want)
		}
	}
}

// TestLimitedTransportCancelsWait proves cancellation ends a Retry-After
// wait instead of sleeping it out.
func TestLimitedTransportCancelsWait(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	header := http.Header{"Retry-After": {"30"}}
	rt := newLimitedTransport(respond(&calls, header, 429), 0, true, nil)
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(50*time.Millisecond, cancel)

	start := time.Now()
	_, err := get(ctx, rt)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("RoundTrip error = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("canceled wait took %v", elapsed)
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1", calls.Load())
	}
}

// TestLimitedTransportClientTimeoutEndsWait proves a limiter wait ends with
// the [http.Client] timeout even for a request carrying [context.Background],
// as go-bitbucket sends.
func TestLimitedTransportClientTimeoutEndsWait(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	rt, _ := newTestTransport(respond(&calls, nil, http.StatusOK), 0.1, true)
	client := &http.Client{Timeout: 100 * time.Millisecond, Transport: rt}
	do := func() error {
		resp, err := client.Get("https://forge.test/api")
		if err == nil {
			_ = resp.Body.Close()
		}
		return err
	}
	if err := do(); err != nil {
		t.Fatalf("first request: %v", err)
	}
	start := time.Now()
	if err := do(); err == nil {
		t.Error("second request within the timeout succeeded, want it to give up waiting")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("wait took %v, want it ended by the client timeout", elapsed)
	}
}

// TestLimitedTransportReplaysBody proves a retried POST, as GitHub's GraphQL
// sends, carries its body again.
func TestLimitedTransportReplaysBody(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var bodies []string
	var calls atomic.Int32
	statuses := respond(&calls, nil, 429, 200)
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(req.Body)
		_ = req.Body.Close()
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
		return statuses(req)
	})
	rt, _ := newTestTransport(base, 0, true)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		"https://forge.test/api/graphql", strings.NewReader(`{"query":"q"}`))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || len(bodies) != 2 || bodies[0] != bodies[1] || bodies[1] == "" {
		t.Errorf("status %d, bodies %q; want 200 and the body sent twice", resp.StatusCode, bodies)
	}
}

// TestProvidersRetryThroughTransport proves the providers' clients are built
// on the transport: GitHub retries a throttled GraphQL POST, and GitLab's own
// retries leave 429 to it, so three attempts stay three.
func TestProvidersRetryThroughTransport(t *testing.T) {
	t.Parallel()

	t.Run("github", func(t *testing.T) {
		t.Parallel()

		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) {
				if calls.Add(1) == 1 {
					w.Header().Set("Retry-After", "0")
					w.WriteHeader(http.StatusTooManyRequests)
					return
				}
				fmt.Fprintf(w,
					`{"data":{"repositoryOwner":{"id":"O_1","login":"acme","repositories":{"nodes":[%s],"pageInfo":{"hasNextPage":false}}}}}`,
					ghRepoNodes(1, 1))
			}))
		defer server.Close()

		p, err := NewGitProvider(t.Context(), domain.ProviderGitHub,
			Options{Token: "tok", BaseURL: server.URL})
		if err != nil {
			t.Fatalf("NewGitProvider: %v", err)
		}
		project := &domain.Project{}
		if err := p.LoadRepos(t.Context(), "acme", project); err != nil {
			t.Fatalf("LoadRepos: %v", err)
		}
		if calls.Load() != 2 || len(project.Repos) != 1 {
			t.Errorf("calls %d, repos %d; want 2 and 1", calls.Load(), len(project.Repos))
		}
	})

	t.Run("gitlab", func(t *testing.T) {
		t.Parallel()

		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
			}))
		defer server.Close()

		p, err := NewGitProvider(t.Context(), domain.ProviderGitLab,
			Options{Token: "tok", BaseURL: server.URL})
		if err != nil {
			t.Fatalf("NewGitProvider: %v", err)
		}
		if err := p.LoadRepos(t.Context(), "acme", &domain.Project{}); err == nil {
			t.Error("LoadRepos against a throttling server = nil error")
		}
		if calls.Load() != maxAttempts {
			t.Errorf("calls = %d, want %d", calls.Load(), maxAttempts)
		}
	})
}
