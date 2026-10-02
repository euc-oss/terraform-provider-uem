package httpclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
)

// trickleHandler serves exactly total bytes, split into chunks writes, each
// followed by a Flush and (except the last) a sleep of interval — a
// slow-but-steady, healthy download.
func trickleHandler(total, chunks int, interval time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", total))
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		chunkSize := total / chunks
		written := 0
		for i := 0; i < chunks; i++ {
			n := chunkSize
			if i == chunks-1 {
				n = total - written
			}
			_, _ = w.Write(bytes.Repeat([]byte{'x'}, n))
			written += n
			if flusher != nil {
				flusher.Flush()
			}
			if i != chunks-1 {
				time.Sleep(interval)
			}
		}
	}
}

// (a) A body trickled slowly enough that total transfer time exceeds the
// SDK's old 30s-equivalent budget (scaled down to 300ms here), but each
// chunk arrives well within the stall window — must succeed.
func TestStallDetection_SlowButSteadyDownloadSucceeds(t *testing.T) {
	const total = 600
	const oldBudget = 300 * time.Millisecond
	srv := httptest.NewServer(trickleHandler(total, 6, 100*time.Millisecond))
	defer srv.Close()

	client := NewClient(Config{HeaderTimeout: time.Second, StallTimeout: 200 * time.Millisecond})

	var gotProgress []int64
	ctx := WithProgress(context.Background(), func(read, _ int64) {
		gotProgress = append(gotProgress, read)
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	elapsed := time.Since(start)

	if len(body) != total {
		t.Fatalf("got %d bytes, want %d", len(body), total)
	}
	if elapsed <= oldBudget {
		t.Fatalf("test setup too fast to be meaningful: elapsed %s <= old budget %s", elapsed, oldBudget)
	}
	if len(gotProgress) == 0 {
		t.Fatal("expected at least one progress callback")
	}
	if gotProgress[len(gotProgress)-1] != total {
		t.Fatalf("final progress read count = %d, want %d", gotProgress[len(gotProgress)-1], total)
	}
}

// (b) The server sends headers plus some bytes, then goes silent — the
// caller's Read must fail with a *StallError carrying accurate byte counts.
func TestStallDetection_MidBodyStallFails(t *testing.T) {
	const sent = 100
	const total = 1000
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", total))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bytes.Repeat([]byte{'x'}, sent))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done() // hang until the client gives up / test ends
	}))
	defer srv.Close()

	client := NewClient(Config{HeaderTimeout: time.Second, StallTimeout: 150 * time.Millisecond})
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error reading headers: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	_, err = io.ReadAll(resp.Body)
	if err == nil {
		t.Fatal("expected a stall error, got nil")
	}
	var stallErr *StallError
	if !errors.As(err, &stallErr) {
		t.Fatalf("expected error to unwrap to *StallError, got %T: %v", err, err)
	}
	if stallErr.BytesRead != sent {
		t.Errorf("BytesRead = %d, want %d", stallErr.BytesRead, sent)
	}
	if stallErr.TotalBytes != total {
		t.Errorf("TotalBytes = %d, want %d", stallErr.TotalBytes, total)
	}
}

// (c) The server delays sending headers past HeaderTimeout — the request
// must fail fast, well before it would time out on its own.
func TestStallDetection_HeaderTimeoutFailsFast(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(Config{HeaderTimeout: 100 * time.Millisecond, StallTimeout: time.Second})
	start := time.Now()
	_, err := client.Get(srv.URL)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a header-timeout error, got nil")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("took %s to fail; expected well under the 2s the server would otherwise take", elapsed)
	}
}

// (d) StallError.Error()'s message format, with and without a known total.
func TestStallError_Message(t *testing.T) {
	cases := []struct {
		name string
		err  *StallError
		want string
	}{
		{
			name: "known total",
			err:  &StallError{BytesRead: 10, TotalBytes: 100, IdleTimeout: 60 * time.Second},
			want: "download stalled: no data received for 1m0s after 10 of 100 bytes",
		},
		{
			name: "unknown total",
			err:  &StallError{BytesRead: 10, TotalBytes: -1, IdleTimeout: 60 * time.Second},
			want: "download stalled: no data received for 1m0s after 10 bytes",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.Error(); got != tc.want {
				t.Errorf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

// Fail-on-revert: reintroducing an overall http.Client.Timeout equal to the
// old (scaled-down) budget must break the same slow-but-steady download that
// TestStallDetection_SlowButSteadyDownloadSucceeds proves succeeds without
// it — demonstrating that the fix's benefit comes from removing the
// wall-clock cap, not from some unrelated change.
func TestStallDetection_RevertCheck_ReintroducedWallClockTimeoutBreaksSteadyDownload(t *testing.T) {
	const total = 600
	const oldBudget = 300 * time.Millisecond
	srv := httptest.NewServer(trickleHandler(total, 6, 100*time.Millisecond))
	defer srv.Close()

	client := NewClient(Config{HeaderTimeout: time.Second, StallTimeout: 200 * time.Millisecond})
	client.Timeout = oldBudget // the exact regression this proves against (SDK client.go: retryClient.HTTPClient.Timeout = config.Timeout)

	resp, err := client.Get(srv.URL)
	if err != nil {
		return // failing here also demonstrates the regression
	}
	defer resp.Body.Close() //nolint:errcheck
	if _, err := io.ReadAll(resp.Body); err == nil {
		t.Fatal("expected the reintroduced wall-clock Timeout to break the slow-but-steady download, but it succeeded")
	}
}

func TestResolveHeaderTimeout(t *testing.T) {
	testResolveTimeoutEnv(t, EnvHeaderTimeout, DefaultHeaderTimeout, ResolveHeaderTimeout)
}

func TestResolveStallTimeout(t *testing.T) {
	testResolveTimeoutEnv(t, EnvStallTimeout, DefaultStallTimeout, ResolveStallTimeout)
}

func testResolveTimeoutEnv(t *testing.T, envName string, def time.Duration, resolve func(LogFunc) time.Duration) {
	t.Helper()

	t.Run("unset uses default", func(t *testing.T) {
		t.Setenv(envName, "")
		os.Unsetenv(envName) //nolint:errcheck // t.Setenv already restores at cleanup
		if got := resolve(nil); got != def {
			t.Errorf("got %s, want default %s", got, def)
		}
	})

	t.Run("valid override is used", func(t *testing.T) {
		t.Setenv(envName, "90s")
		if got := resolve(nil); got != 90*time.Second {
			t.Errorf("got %s, want 90s", got)
		}
	})

	t.Run("invalid value falls back and logs", func(t *testing.T) {
		t.Setenv(envName, "not-a-duration")
		var logged string
		got := resolve(func(format string, args ...any) { logged = fmt.Sprintf(format, args...) })
		if got != def {
			t.Errorf("got %s, want default %s", got, def)
		}
		if logged == "" {
			t.Error("expected a log call for an invalid override")
		}
	})

	t.Run("non-positive value falls back and logs", func(t *testing.T) {
		t.Setenv(envName, "0s")
		var logged string
		got := resolve(func(format string, args ...any) { logged = fmt.Sprintf(format, args...) })
		if got != def {
			t.Errorf("got %s, want default %s", got, def)
		}
		if logged == "" {
			t.Error("expected a log call for a non-positive override")
		}
	})
}

func TestNewClient_TransportIsStallAware(t *testing.T) {
	c := NewClient(Config{HeaderTimeout: time.Second, StallTimeout: time.Second})
	if c.Timeout != 0 {
		t.Errorf("client.Timeout = %s, want 0 (no overall wall-clock cap)", c.Timeout)
	}
	if _, ok := c.Transport.(*Transport); !ok {
		t.Fatalf("client.Transport is %T, want *Transport", c.Transport)
	}
}

// TestNewClient_PreservesProxyAndHeaderTimeout: the cloned DefaultTransport
// keeps ProxyFromEnvironment (corporate proxies keep working) and carries the
// configured ResponseHeaderTimeout.
func TestNewClient_PreservesProxyAndHeaderTimeout(t *testing.T) {
	c := NewClient(Config{HeaderTimeout: 7 * time.Second, StallTimeout: time.Second})
	tr, ok := c.Transport.(*Transport)
	if !ok {
		t.Fatalf("Transport is %T", c.Transport)
	}
	inner, ok := tr.wrapped.(*http.Transport)
	if !ok {
		t.Fatalf("wrapped is %T", tr.wrapped)
	}
	if inner.Proxy == nil {
		t.Error("Proxy is nil; ProxyFromEnvironment was dropped")
	}
	if inner.ResponseHeaderTimeout != 7*time.Second {
		t.Errorf("ResponseHeaderTimeout = %s, want 7s", inner.ResponseHeaderTimeout)
	}
	if inner.TLSHandshakeTimeout <= 0 || inner.DialContext == nil {
		t.Error("TLS handshake / dial timeouts not set")
	}
}

type blockingBody struct {
	unblock chan struct{}
	once    sync.Once
}

func (b *blockingBody) Read([]byte) (int, error) { <-b.unblock; return 0, io.ErrClosedPipe }
func (b *blockingBody) Close() error             { b.once.Do(func() { close(b.unblock) }); return nil }

// TestStallReader_CloseThenReadAndIdle: Read after Close returns an error (no
// panic, no StallError), and the idle timer never reports a stall once the
// caller has closed the body — run under -race for the Close/Read/onIdle race.
func TestStallReader_CloseThenReadAndIdle(t *testing.T) {
	b := &blockingBody{unblock: make(chan struct{})}
	sr := newStallReader(b, 20*time.Millisecond, -1, nil)
	done := make(chan error, 1)
	go func() { _, err := sr.Read(make([]byte, 1)); done <- err }()
	_ = sr.Close()
	if err := <-done; err == nil {
		t.Fatal("Read racing Close returned nil error")
	}
	time.Sleep(50 * time.Millisecond)
	_, err := sr.Read(make([]byte, 1))
	var se *StallError
	if err == nil || errors.As(err, &se) {
		t.Fatalf("Read after Close = %v, want a non-stall error", err)
	}
}
