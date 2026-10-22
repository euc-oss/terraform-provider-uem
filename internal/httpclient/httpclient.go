// Package httpclient builds the stall-aware *http.Client the provider uses
// for every SDK client it constructs (internal-ticket / B10).
//
// Background: the pinned terraform-sdk-uem client (client/client.go)
// defaults Config.Timeout to 30s whenever it is left zero, and applies that
// as http.Client.Timeout — a single wall-clock cap over the ENTIRE request,
// headers and body alike. A caller-supplied Config.HTTPClient is accepted,
// but the SAME code always overwrites its Timeout field with Config.Timeout
// (only the Transport is left alone). For a large uem_mac_application
// binary download (BlobsV2Service.Get, which the SDK buffers whole via
// io.ReadAll), that 30s cap fires on any file that legitimately takes
// longer than 30 seconds to transfer, not just on a truly stalled
// connection — and retryablehttp's own retry logic never gets a chance to
// help, since a mid-body read failure happens in client.go's
// handleResponse AFTER retryablehttp's Do() has already returned
// successfully with the response headers.
//
// The fix here does not touch the SDK (it is a pinned, external
// dependency): this package gives the provider a *http.Client with NO
// overall wall-clock Timeout (so it survives arbitrarily large,
// slow-but-healthy transfers), and instead detects a genuinely stalled
// connection at the Transport level:
//
//   - ResponseHeaderTimeout bounds how long the SERVER may take to start
//     responding at all — a legitimately slow but healthy server should
//     still answer headers quickly even when the body it is about to send
//     is large.
//   - An idle-read timeout on the response BODY: every Read() that returns
//     at least one byte resets an idle timer; if no data arrives for the
//     configured window, the underlying body is closed so the blocked
//     Read() returns promptly with a typed *StallError, instead of hanging
//     forever or (pre-fix) being killed at an arbitrary, non-diagnostic
//     point by the wall-clock Timeout.
//
// Callers still set the SDK's own Config.Timeout field (see
// internal/provider/provider.go) — to a long fixed value, purely so the
// SDK's zero-defaults-to-30s branch never substitutes a real cap back in.
// It is not expected to ever fire in practice; the header/stall timeouts
// below are what actually protect a caller from hanging indefinitely.
package httpclient

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	// DefaultHeaderTimeout is used when UEM_HTTP_HEADER_TIMEOUT is unset or
	// invalid.
	DefaultHeaderTimeout = 60 * time.Second
	// DefaultStallTimeout is used when UEM_HTTP_STALL_TIMEOUT is unset or
	// invalid.
	DefaultStallTimeout = 60 * time.Second

	// EnvHeaderTimeout names the environment variable overriding
	// DefaultHeaderTimeout, as a Go duration string (e.g. "90s", "2m").
	EnvHeaderTimeout = "UEM_HTTP_HEADER_TIMEOUT"
	// EnvStallTimeout names the environment variable overriding
	// DefaultStallTimeout, as a Go duration string.
	EnvStallTimeout = "UEM_HTTP_STALL_TIMEOUT"
)

// dialTimeout, dialKeepAlive, and tlsHandshakeTimeout are sensible, fixed
// connection-setup timeouts — unlike HeaderTimeout/StallTimeout, B10's design
// does not call for these to be independently env-tunable.
const (
	dialTimeout         = 30 * time.Second
	dialKeepAlive       = 30 * time.Second
	tlsHandshakeTimeout = 30 * time.Second
)

// LogFunc receives a printf-style message when an env override is invalid
// (or absent) and a default is used instead.
type LogFunc func(format string, args ...any)

// ResolveHeaderTimeout reads EnvHeaderTimeout, falling back to
// DefaultHeaderTimeout when the variable is unset, non-positive, or not a
// valid Go duration. Logf (nil-safe) is called with the reason whenever the
// fallback is taken because an explicit value was present but invalid.
func ResolveHeaderTimeout(logf LogFunc) time.Duration {
	return resolveDuration(EnvHeaderTimeout, DefaultHeaderTimeout, logf)
}

// ResolveStallTimeout is ResolveHeaderTimeout's counterpart for
// EnvStallTimeout / DefaultStallTimeout.
func ResolveStallTimeout(logf LogFunc) time.Duration {
	return resolveDuration(EnvStallTimeout, DefaultStallTimeout, logf)
}

func resolveDuration(envName string, def time.Duration, logf LogFunc) time.Duration {
	raw := os.Getenv(envName)
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		if logf != nil {
			logf("%s=%q is not a valid Go duration (%v); using default %s", envName, raw, err, def)
		}
		return def
	}
	if d <= 0 {
		if logf != nil {
			logf("%s=%q must be a positive duration; using default %s", envName, raw, def)
		}
		return def
	}
	return d
}

// Config configures NewClient.
type Config struct {
	// HeaderTimeout bounds how long the server may take to send response
	// headers (net/http.Transport.ResponseHeaderTimeout).
	HeaderTimeout time.Duration
	// StallTimeout is the idle-read window applied to every response body:
	// a Read() gap longer than this closes the body and surfaces a
	// *StallError to the reader.
	StallTimeout time.Duration
}

// NewClient builds a *http.Client with NO overall Timeout (see package doc)
// and a Transport that enforces cfg.HeaderTimeout for response headers and
// cfg.StallTimeout as an idle-read watchdog on every response body.
func NewClient(cfg Config) *http.Client {
	base := http.DefaultTransport.(*http.Transport).Clone() //nolint:forcetypeassert // http.DefaultTransport is always *http.Transport
	base.ResponseHeaderTimeout = cfg.HeaderTimeout
	base.TLSHandshakeTimeout = tlsHandshakeTimeout
	base.DialContext = (&net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: dialKeepAlive,
	}).DialContext

	return &http.Client{
		Transport: &Transport{wrapped: base, stallTimeout: cfg.StallTimeout},
	}
}

// Transport wraps another http.RoundTripper (normally a *http.Transport
// clone) and replaces every non-nil response body with one that enforces an
// idle-read timeout (see stallReader). It is exported so callers/tests can
// confirm a *http.Client was built by this package, e.g.
// `_, ok := client.Transport.(*httpclient.Transport)`.
type Transport struct {
	wrapped      http.RoundTripper
	stallTimeout time.Duration
}

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.wrapped.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil {
		return resp, err
	}
	progress, _ := progressFromContext(req.Context())
	resp.Body = newStallReader(resp.Body, t.stallTimeout, resp.ContentLength, progress)
	return resp, nil
}

// StallError is returned from a response body Read() once StallTimeout has
// elapsed with no data received. BytesRead is how much of the body had
// already been read before the stall; TotalBytes is the response's
// Content-Length, or -1 when the server did not send one.
type StallError struct {
	BytesRead   int64
	TotalBytes  int64
	IdleTimeout time.Duration
}

// Error implements error.
func (e *StallError) Error() string {
	if e.TotalBytes >= 0 {
		return fmt.Sprintf("download stalled: no data received for %s after %d of %d bytes", e.IdleTimeout, e.BytesRead, e.TotalBytes)
	}
	return fmt.Sprintf("download stalled: no data received for %s after %d bytes", e.IdleTimeout, e.BytesRead)
}

// stallReader wraps a response body and closes it once StallTimeout elapses
// between Reads that make progress (n > 0). Closing an in-flight Read
// unblocks it — net/http's body implementations return promptly once the
// underlying connection is closed — letting Read itself return the typed
// StallError below, rather than the caller hanging indefinitely or (pre-B10)
// the whole client.Timeout wall clock firing at an arbitrary point during a
// healthy-but-large transfer.
type stallReader struct {
	rc       io.ReadCloser
	timeout  time.Duration
	total    int64 // -1 when unknown (no Content-Length)
	progress ProgressFunc

	timer *time.Timer

	mu      sync.Mutex
	read    int64
	stalled bool
	closed  bool
}

func newStallReader(rc io.ReadCloser, timeout time.Duration, total int64, progress ProgressFunc) *stallReader {
	sr := &stallReader{rc: rc, timeout: timeout, total: total, progress: progress}
	if timeout > 0 {
		sr.timer = time.AfterFunc(timeout, sr.onIdle)
	}
	return sr
}

// onIdle fires when StallTimeout has elapsed without a successful Read. It
// force-closes the underlying body so any Read blocked on it returns.
func (sr *stallReader) onIdle() {
	sr.mu.Lock()
	if sr.closed {
		sr.mu.Unlock()
		return
	}
	sr.stalled = true
	sr.mu.Unlock()
	_ = sr.rc.Close()
}

// Read implements io.Reader.
func (sr *stallReader) Read(p []byte) (int, error) {
	n, err := sr.rc.Read(p)
	if n > 0 {
		sr.mu.Lock()
		sr.read += int64(n)
		read := sr.read
		sr.mu.Unlock()
		if sr.timer != nil {
			sr.timer.Reset(sr.timeout)
		}
		if sr.progress != nil {
			sr.progress(read, sr.total)
		}
	}
	if err != nil {
		sr.mu.Lock()
		stalled := sr.stalled
		read := sr.read
		total := sr.total
		sr.mu.Unlock()
		if stalled {
			return n, &StallError{BytesRead: read, TotalBytes: total, IdleTimeout: sr.timeout}
		}
	}
	return n, err
}

// Close implements io.Closer.
func (sr *stallReader) Close() error {
	sr.mu.Lock()
	sr.closed = true
	sr.mu.Unlock()
	if sr.timer != nil {
		sr.timer.Stop()
	}
	return sr.rc.Close()
}

// ProgressFunc is invoked as a response body is read, with the cumulative
// bytes read so far and the total (-1 if unknown, i.e. no Content-Length).
type ProgressFunc func(read, total int64)

type progressCtxKey struct{}

// WithProgress returns a context carrying fn, so the next request issued
// with it (through a *http.Client built by NewClient) reports byte progress
// via fn as its response body is read. Only one callback is honored per
// request context.
func WithProgress(ctx context.Context, fn ProgressFunc) context.Context {
	return context.WithValue(ctx, progressCtxKey{}, fn)
}

func progressFromContext(ctx context.Context) (ProgressFunc, bool) {
	fn, ok := ctx.Value(progressCtxKey{}).(ProgressFunc)
	return fn, ok
}

// ProgressFromContext reports whether ctx carries a progress callback set by
// WithProgress. Exported for tests (e.g. confirming a callback was scoped to
// the right request), not needed by RoundTrip itself (which uses the
// unexported progressFromContext directly).
func ProgressFromContext(ctx context.Context) (ProgressFunc, bool) {
	return progressFromContext(ctx)
}
