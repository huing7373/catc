// Command ws_loadgen is the Story 0.15 (Spike-OP1) one-off WS load generator.
// It opens N concurrent WebSocket connections against an /ws endpoint running
// in debug mode and drives one of three scenarios (cold_connect / raise_wrist
// / long_lived) to produce p50/p95/p99 latency samples and per-error-category
// counts as a JSON summary.
//
// # Scope limit — this is a hub-side stress tool, NOT an AC5 NFR-REL-4 probe
//
// All three scenarios measure dial → first debug.echo round-trip. The AC5
// cold-start / reconnect metric in `docs/spikes/op1-ws-stability.md` §5 is
// defined as "TCP connect + WS upgrade + first session.resume.result" end-to-
// end latency on a real Apple Watch. This tool deliberately does NOT exercise
// the session.resume path (Story 0.15 Dev Notes 禁止事项: session.resume pulls
// in the provider fan-out which is a different measurement axis). So AC5 cells
// in §3 / §5 / §6 MUST be filled by real-device data — ws_loadgen numbers are
// a Hub-side lower bound, only appropriate for §7 (AC6 / ADR-003) hub load.
//
// Scenarios:
//
//   - cold_connect: each worker opens a WS, sends one debug.echo, closes, and
//     repeats until -duration elapses. Useful for hub-side acceptance-loop
//     stress. NOT a substitute for AC5 cold-start (real Watch + session.resume).
//
//   - raise_wrist: like cold_connect but with a random 1–5 s sleep between
//     cycles. Exercises the Story 0.11 reconnect rate limiter (5 req / 60 s
//     per user by default). NOT a substitute for AC5 raise-wrist reconnect
//     (real Watch firing WKExtendedRuntimeSession + session.resume).
//
//   - long_lived: each worker opens a WS once and emits debug.echo every
//     -send-interval until -duration elapses. On any read/write error the
//     worker reconnects (with a small backoff) so the requested N is held
//     steady throughout. This is the scenario matching AC6 hub load test —
//     see ReconnectAttempts in the summary for stability observability.
//
// Design constraints (Story 0.15 AC12 + backend-architecture-guide.md §19):
//
//   - no new third-party dependencies: only stdlib + gorilla/websocket +
//     google/uuid + rs/zerolog + internal/dto already in server/go.mod
//   - no fmt.Printf / log.Printf: minimal zerolog to stderr; JSON summary
//     written to stdout or -report path via encoding/json
//   - every worker defers conn.Close() to avoid fd leaks
//   - root context uses signal.NotifyContext(ctx, os.Interrupt, SIGTERM)
//     so Ctrl-C produces a partial summary rather than losing samples
//   - dto.WSMessagesByType["debug.echo"] lookup at startup; if Story 0.14
//     ever removes the entry, the tool panics immediately rather than
//     silently sending an envelope the server rejects as UNKNOWN_MESSAGE_TYPE
//
// This tool is intentionally thrown away after Spike-OP1 converges — it lives
// under server/tools/ where one-off scripts belong (architecture §Project
// Structure). It does not provide the automatic session.resume path; that is
// the job of a full Watch/iPhone client simulator, out of scope here.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"github.com/huing/cat/server/internal/dto"
)

const (
	scenarioColdConnect = "cold_connect"
	scenarioRaiseWrist  = "raise_wrist"
	scenarioLongLived   = "long_lived"

	debugEchoType = "debug.echo"
)

// Config captures the parsed command-line flags. Exported fields make the
// JSON summary self-describing (the inputs that produced the numbers are
// recorded alongside the numbers themselves).
type Config struct {
	URL          string `json:"url"`
	Concurrent   int    `json:"concurrent"`
	DurationMs   int64  `json:"durationMs"`
	SendIntervalMs int64 `json:"sendIntervalMs"`
	Scenario     string `json:"scenario"`
	TokenPrefix  string `json:"tokenPrefix"`
}

// Percentiles holds p50 / p95 / p99 in milliseconds.
type Percentiles struct {
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	P99 float64 `json:"p99"`
}

// ErrorCounts groups per-category failure counts observed during a run.
type ErrorCounts struct {
	Dial     int `json:"dial"`
	Upgrade  int `json:"upgrade"`
	Write    int `json:"write"`
	Read     int `json:"read"`
	Parse    int `json:"parse"`
	Mismatch int `json:"mismatch"`
}

// Summary is the final JSON record written to -report (or stdout).
//
// ReconnectAttempts counts long_lived outer-loop iterations that happen AFTER
// a worker has had at least one successful session. Each increment represents
// one "reconnect attempt"; that attempt itself may succeed (session runs
// again) or fail (dial error / rate limit) — both count. Pre-first-success
// retries are deliberately excluded (a worker that never gets an initial
// connect contributes 0), so the counter is scoped to "how much session
// churn the run experienced", not "how many dials were tried".
//
// The documented stability judgement in docs/spikes/op1-ws-stability.md §7
// uses a per-worker denominator, NOT ConnectSuccess (which is inflated by
// successful reconnects themselves, so using it as the denominator
// systematically under-reports churn by ~R/(N+R) where R is reconnects):
//
//	reconnectRatio = ReconnectAttempts / Config.Concurrent
//
// > 0.05 means the run averaged more than 1 reconnect per 20 workers.
// Above that threshold the configured N was not held steady; p95/p99
// readings must be labelled non-steady and cannot feed the ADR-003
// broadcastLatencyP99 ≤ 3 s gate directly.
type Summary struct {
	StartedAt         time.Time   `json:"startedAt"`
	FinishedAt        time.Time   `json:"finishedAt"`
	Config            Config      `json:"config"`
	ConnectSuccess    int64       `json:"connectSuccess"`
	ConnectFailures   int64       `json:"connectFailures"`
	ReconnectAttempts int64       `json:"reconnectAttempts"`
	EchoSamples       int64       `json:"echoSamples"`
	ConnectLatencyMs  Percentiles `json:"connectLatencyMs"`
	EchoRttMs         Percentiles `json:"echoRttMs"`
	Errors            ErrorCounts `json:"errors"`
}

// errCounter is a goroutine-safe bucket for categorised failure counts. The
// mutex approach is overkill for the low rates involved but keeps the code
// obvious — the alternative (atomic.Int64 per field) would need a method per
// category which buys nothing.
type errCounter struct {
	mu     sync.Mutex
	counts ErrorCounts
}

// Inc increments the named bucket. Unknown kinds are silently ignored so
// adding new error categories stays a one-line change.
func (e *errCounter) Inc(kind string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch kind {
	case "dial":
		e.counts.Dial++
	case "upgrade":
		e.counts.Upgrade++
	case "write":
		e.counts.Write++
	case "read":
		e.counts.Read++
	case "parse":
		e.counts.Parse++
	case "mismatch":
		e.counts.Mismatch++
	}
}

type flags struct {
	url          string
	concurrent   int
	duration     time.Duration
	sendInterval time.Duration
	scenario     string
	reportPath   string
	tokenPrefix  string
	verbose      bool
}

const usageText = `ws_loadgen — Story 0.15 WS load generator (one-off tool)

Usage:
  ws_loadgen [flags]

Flags:
  -url string            WS endpoint URL (default "ws://127.0.0.1:8080/ws")
  -concurrent int        number of concurrent worker goroutines (default 10)
  -duration duration     total test duration, Go syntax e.g. "60s", "10m" (default 60s)
  -send-interval duration
                         interval between echo sends per worker in long_lived (default 1s)
  -scenario string       one of: cold_connect | raise_wrist | long_lived (default "long_lived")
  -report string         path for JSON summary; empty means stdout (default "")
  -token-prefix string   bearer-token/userID prefix; worker i uses "<prefix>i" (default "loadgen-")
  -verbose               enable per-worker debug logs to stderr

Examples:
  ws_loadgen -concurrent 10 -duration 60s -scenario cold_connect
  ws_loadgen -concurrent 1000 -duration 10m -scenario long_lived -report hub-1k.json
  ws_loadgen -concurrent 50 -duration 5m -scenario raise_wrist
`

func parseFlags() (flags, error) {
	var f flags
	flag.StringVar(&f.url, "url", "ws://127.0.0.1:8080/ws", "WS endpoint URL")
	flag.IntVar(&f.concurrent, "concurrent", 10, "number of concurrent workers")
	flag.DurationVar(&f.duration, "duration", 60*time.Second, "total test duration")
	flag.DurationVar(&f.sendInterval, "send-interval", time.Second, "interval between echoes in long_lived scenario")
	flag.StringVar(&f.scenario, "scenario", scenarioLongLived, "cold_connect | raise_wrist | long_lived")
	flag.StringVar(&f.reportPath, "report", "", "path for JSON summary (empty = stdout)")
	flag.StringVar(&f.tokenPrefix, "token-prefix", "loadgen-", "bearer-token/userID prefix")
	flag.BoolVar(&f.verbose, "verbose", false, "enable per-worker debug logs")
	flag.Usage = func() { _, _ = io.WriteString(os.Stderr, usageText) }
	flag.Parse()

	switch f.scenario {
	case scenarioColdConnect, scenarioRaiseWrist, scenarioLongLived:
	default:
		return f, errors.New("-scenario must be one of cold_connect | raise_wrist | long_lived")
	}
	if f.concurrent < 1 {
		return f, errors.New("-concurrent must be >= 1")
	}
	if f.duration <= 0 {
		return f, errors.New("-duration must be > 0")
	}
	if f.sendInterval <= 0 {
		return f, errors.New("-send-interval must be > 0")
	}
	return f, nil
}

func main() {
	f, err := parseFlags()
	if err != nil {
		bootLogger := zerolog.New(os.Stderr).With().Timestamp().Logger()
		bootLogger.Error().Err(err).Msg("invalid flags")
		flag.Usage()
		os.Exit(2)
	}

	level := zerolog.InfoLevel
	if f.verbose {
		level = zerolog.DebugLevel
	}
	logger := zerolog.New(os.Stderr).Level(level).With().Timestamp().Str("tool", "ws_loadgen").Logger()

	if _, ok := dto.WSMessagesByType[debugEchoType]; !ok {
		logger.Fatal().Str("type", debugEchoType).Msg("dto.WSMessagesByType drift: debug.echo missing — Story 0.14 registry out of sync")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	summary, err := run(ctx, f, logger)
	if err != nil {
		logger.Error().Err(err).Msg("run failed")
		os.Exit(1)
	}

	if err := writeSummary(summary, f.reportPath); err != nil {
		logger.Error().Err(err).Msg("failed to write summary")
		os.Exit(1)
	}

	logger.Info().
		Str("scenario", summary.Config.Scenario).
		Int("concurrent", summary.Config.Concurrent).
		Int64("durationMs", summary.Config.DurationMs).
		Int64("connectSuccess", summary.ConnectSuccess).
		Int64("connectFailures", summary.ConnectFailures).
		Int64("reconnectAttempts", summary.ReconnectAttempts).
		Int64("echoSamples", summary.EchoSamples).
		Msg("run complete")
}

// run orchestrates the worker pool for the chosen scenario and aggregates
// their samples into a Summary. It returns once all workers exit, whether
// because -duration elapsed or because ctx was cancelled (Ctrl-C).
func run(ctx context.Context, f flags, logger zerolog.Logger) (Summary, error) {
	startedAt := time.Now()

	runCtx, cancel := context.WithTimeout(ctx, f.duration)
	defer cancel()

	var (
		wg         sync.WaitGroup
		connectOK  atomic.Int64
		connectErr atomic.Int64
		reconnects atomic.Int64
		echoCount  atomic.Int64
	)
	errs := &errCounter{}

	connectLatBuf := newSampleBuffer()
	echoRttBuf := newSampleBuffer()

	for i := 0; i < f.concurrent; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			token := f.tokenPrefix + itoa(workerID)
			wl := logger.With().Int("worker", workerID).Str("userId", token).Logger()
			switch f.scenario {
			case scenarioColdConnect:
				workerColdConnect(runCtx, f, wl, token, connectLatBuf, echoRttBuf, &connectOK, &connectErr, &echoCount, errs)
			case scenarioRaiseWrist:
				workerRaiseWrist(runCtx, f, wl, token, connectLatBuf, echoRttBuf, &connectOK, &connectErr, &echoCount, errs)
			case scenarioLongLived:
				workerLongLived(runCtx, f, wl, token, connectLatBuf, echoRttBuf, &connectOK, &connectErr, &echoCount, &reconnects, errs)
			}
		}(i)
	}

	wg.Wait()
	finishedAt := time.Now()

	return Summary{
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Config: Config{
			URL:            f.url,
			Concurrent:     f.concurrent,
			DurationMs:     f.duration.Milliseconds(),
			SendIntervalMs: f.sendInterval.Milliseconds(),
			Scenario:       f.scenario,
			TokenPrefix:    f.tokenPrefix,
		},
		ConnectSuccess:    connectOK.Load(),
		ConnectFailures:   connectErr.Load(),
		ReconnectAttempts: reconnects.Load(),
		EchoSamples:       echoCount.Load(),
		ConnectLatencyMs:  percentiles(connectLatBuf.drain()),
		EchoRttMs:         percentiles(echoRttBuf.drain()),
		Errors:            errs.snapshot(),
	}, nil
}

// workerColdConnect opens one WS per iteration, sends one debug.echo, and
// closes. Measures hub-side dial + debug.echo RTT under repeated open/close
// pressure. NOT the AC5 cold-start metric (that requires a real Watch
// issuing session.resume as the first business message — see package doc).
func workerColdConnect(
	ctx context.Context,
	f flags,
	logger zerolog.Logger,
	token string,
	connectLat, echoRtt *sampleBuffer,
	okCount, errCount, echoCount *atomic.Int64,
	errs *errCounter,
) {
	for ctx.Err() == nil {
		if !runOneCycle(ctx, f, logger, token, connectLat, echoRtt, okCount, errCount, echoCount, errs) {
			return
		}
	}
}

// workerRaiseWrist mirrors cold_connect but sleeps a random 1–5 s between
// cycles to match the watchOS raise-wrist cadence spec. The same WS reconnect
// guard (Story 0.11 rate limiter) applies here — a tight loop without the
// sleep would be indistinguishable from a J4 reconnect storm. Like
// cold_connect this is a hub-side stress scenario and NOT the AC5 raise-wrist
// reconnect metric (that requires a real Watch firing WKExtendedRuntimeSession
// and issuing session.resume — see package doc).
func workerRaiseWrist(
	ctx context.Context,
	f flags,
	logger zerolog.Logger,
	token string,
	connectLat, echoRtt *sampleBuffer,
	okCount, errCount, echoCount *atomic.Int64,
	errs *errCounter,
) {
	for ctx.Err() == nil {
		if !runOneCycle(ctx, f, logger, token, connectLat, echoRtt, okCount, errCount, echoCount, errs) {
			return
		}
		// rand.Intn is fine for a one-off tool — no cryptographic property
		// required; we just want jitter to avoid lockstep reconnects.
		jitter := time.Duration(1000+rand.Intn(4000)) * time.Millisecond
		select {
		case <-ctx.Done():
			return
		case <-time.After(jitter):
		}
	}
}

// reconnectBackoff is how long a long_lived worker waits before re-dialing
// after a read / write error or a failed dial. Short enough that a 10-minute
// run with transient blips still spends most of its time connected;
// long enough that a fully-down server or a rate-limiter-blocked user does
// not trigger a tight spin that distorts CPU and fd numbers in the summary.
const reconnectBackoff = 250 * time.Millisecond

// workerLongLived holds one logical WS session for the full -duration window,
// reconnecting whenever the underlying connection fails so the requested N is
// not eroded by transient read / write / dial errors. Each successful dial
// increments okCount and contributes a connectLat sample; each failed dial
// increments errCount and classifies via classifyDialError. Echo RTTs are
// recorded per successful .result. reconnects counts outer-loop cycles so the
// Summary exposes how stable the run was (see Summary.ReconnectAttempts).
//
// This is the scenario to use for AC6 (Hub 1k/3k/5k/10k steady-state): if the
// server terminates a session (rate limit, overload, network blip), the worker
// re-establishes and resumes sending. Without this, a single transient error
// would silently collapse N and make p95/p99 look much better than the real
// steady-state capacity — which was the exact reviewer finding in Story 0.15
// Round 1.
func workerLongLived(
	ctx context.Context,
	f flags,
	logger zerolog.Logger,
	token string,
	connectLat, echoRtt *sampleBuffer,
	okCount, errCount, echoCount, reconnects *atomic.Int64,
	errs *errCounter,
) {
	// everSucceeded flips to true the first time this worker's dial lands a
	// usable session. Only iterations after that first success count as
	// reconnect attempts — a worker that never manages an initial connect
	// contributes 0 to ReconnectAttempts, matching the Summary godoc.
	everSucceeded := false
	for ctx.Err() == nil {
		if everSucceeded {
			reconnects.Add(1)
		}
		connected := runLongLivedSession(ctx, f, logger, token, connectLat, echoRtt, okCount, errCount, echoCount, errs)
		if connected {
			everSucceeded = true
		}
		// Backoff before the next iteration regardless of outcome: a failed
		// dial retry without a pause would spin the CPU and hammer the
		// Story 0.11 rate limiter; a successful session that just ended
		// should not reconnect instantaneously either, since most real
		// disconnect causes (network blip, rate-limit window) are relieved
		// by a small wait.
		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnectBackoff):
		}
	}
}

// runLongLivedSession is one dial → echo-loop → teardown cycle. Returns true
// iff the dial succeeded (so the outer worker can distinguish "initial-
// connect retry" from "post-success reconnect" for the ReconnectAttempts
// metric). Errors are accounted for via the shared counters.
func runLongLivedSession(
	ctx context.Context,
	f flags,
	logger zerolog.Logger,
	token string,
	connectLat, echoRtt *sampleBuffer,
	okCount, errCount, echoCount *atomic.Int64,
	errs *errCounter,
) bool {
	conn, connMs, err := dial(ctx, f.url, token)
	if err != nil {
		errCount.Add(1)
		classifyDialError(err, errs)
		logger.Debug().Err(err).Msg("dial failed; will retry after backoff")
		return false
	}
	defer conn.Close()
	okCount.Add(1)
	connectLat.add(connMs)
	logger.Debug().Float64("connectMs", connMs).Msg("connected")

	ticker := time.NewTicker(f.sendInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return true
		case <-ticker.C:
			rtt, err := sendEcho(conn)
			if err != nil {
				classifyEchoError(err, errs)
				logger.Debug().Err(err).Msg("echo failed; will reconnect")
				return true
			}
			echoRtt.add(rtt)
			echoCount.Add(1)
		}
	}
}

// runOneCycle performs dial → one debug.echo → close. Used by cold_connect
// and raise_wrist. Returns false when ctx is done so the worker loop exits
// cleanly instead of racing the timeout.
func runOneCycle(
	ctx context.Context,
	f flags,
	logger zerolog.Logger,
	token string,
	connectLat, echoRtt *sampleBuffer,
	okCount, errCount, echoCount *atomic.Int64,
	errs *errCounter,
) bool {
	if ctx.Err() != nil {
		return false
	}
	conn, connMs, err := dial(ctx, f.url, token)
	if err != nil {
		errCount.Add(1)
		classifyDialError(err, errs)
		logger.Debug().Err(err).Msg("dial failed")
		return ctx.Err() == nil
	}
	defer conn.Close()
	okCount.Add(1)
	connectLat.add(connMs)

	rtt, err := sendEcho(conn)
	if err != nil {
		classifyEchoError(err, errs)
		logger.Debug().Err(err).Msg("echo failed")
		return ctx.Err() == nil
	}
	echoRtt.add(rtt)
	echoCount.Add(1)
	return true
}

// dial performs a WS handshake with Authorization: Bearer <token>. In debug
// mode (Story 0.11 NewDebugValidator), any non-empty token is accepted and
// used verbatim as the userID, so worker i becomes "<prefix>i". The returned
// latency is the full TCP→TLS→HTTP upgrade window; it does not include the
// first echo.
func dial(ctx context.Context, url, token string) (*websocket.Conn, float64, error) {
	start := time.Now()
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	conn, resp, err := dialer.DialContext(ctx, url, header)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, 0, err
	}
	ms := float64(time.Since(start).Microseconds()) / 1000.0
	return conn, ms, nil
}

// sendEcho writes one debug.echo envelope with a fresh UUID and blocks for
// the matching debug.echo.result response. Envelope.id is a fresh UUID per
// send (Story 0.10 dedup key is length-prefix-encoded so UUID-length ids
// avoid any delimiter collision, though debug.echo is non-dedup and wouldn't
// collide anyway). Read deadline is 10 s — long enough to survive one ping
// interval under severe load but short enough to surface hung connections.
func sendEcho(conn *websocket.Conn) (float64, error) {
	id := uuid.NewString()
	env := map[string]any{
		"id":      id,
		"type":    debugEchoType,
		"payload": map[string]any{"t": time.Now().UnixNano()},
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return 0, err
	}

	start := time.Now()
	if err := conn.WriteMessage(websocket.TextMessage, raw); err != nil {
		return 0, writeErr{err: err}
	}

	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		return 0, readErr{err: err}
	}

	var resp struct {
		ID   string `json:"id"`
		OK   bool   `json:"ok"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return 0, parseErr{err: err}
	}
	if resp.ID != id {
		return 0, mismatchErr{}
	}
	return float64(time.Since(start).Microseconds()) / 1000.0, nil
}

// --- error classification (purely for the ErrorCounts summary field) -----

type writeErr struct{ err error }

func (e writeErr) Error() string { return "write: " + e.err.Error() }
func (e writeErr) Unwrap() error { return e.err }

type readErr struct{ err error }

func (e readErr) Error() string { return "read: " + e.err.Error() }
func (e readErr) Unwrap() error { return e.err }

type parseErr struct{ err error }

func (e parseErr) Error() string { return "parse: " + e.err.Error() }
func (e parseErr) Unwrap() error { return e.err }

type mismatchErr struct{}

func (mismatchErr) Error() string { return "echo response id mismatch" }

func classifyDialError(err error, errs *errCounter) {
	if _, ok := err.(*websocket.CloseError); ok {
		errs.Inc("upgrade")
		return
	}
	if errors.Is(err, websocket.ErrBadHandshake) {
		errs.Inc("upgrade")
		return
	}
	errs.Inc("dial")
}

func classifyEchoError(err error, errs *errCounter) {
	var (
		werr writeErr
		rerr readErr
		perr parseErr
		merr mismatchErr
	)
	switch {
	case errors.As(err, &werr):
		errs.Inc("write")
	case errors.As(err, &rerr):
		errs.Inc("read")
	case errors.As(err, &perr):
		errs.Inc("parse")
	case errors.As(err, &merr):
		errs.Inc("mismatch")
	default:
		errs.Inc("read")
	}
}

// --- concurrent sample buffer (mutex-guarded slice) -----------------------

type sampleBuffer struct {
	mu   sync.Mutex
	data []float64
}

func newSampleBuffer() *sampleBuffer { return &sampleBuffer{data: make([]float64, 0, 1024)} }

func (b *sampleBuffer) add(v float64) {
	b.mu.Lock()
	b.data = append(b.data, v)
	b.mu.Unlock()
}

func (b *sampleBuffer) drain() []float64 {
	b.mu.Lock()
	out := b.data
	b.data = nil
	b.mu.Unlock()
	return out
}

// percentiles computes p50/p95/p99 via sort + index (nearest-rank method).
// Returns zero-valued Percentiles on empty input — no NaN, no panic.
func percentiles(samples []float64) Percentiles {
	if len(samples) == 0 {
		return Percentiles{}
	}
	sort.Float64s(samples)
	return Percentiles{
		P50: samples[rank(len(samples), 0.50)],
		P95: samples[rank(len(samples), 0.95)],
		P99: samples[rank(len(samples), 0.99)],
	}
}

// rank returns the nearest-rank index for a given percentile p ∈ (0,1] over n
// samples. n must be ≥ 1; callers gate on empty input above.
func rank(n int, p float64) int {
	idx := int(float64(n)*p + 0.5)
	if idx >= n {
		idx = n - 1
	}
	if idx < 0 {
		idx = 0
	}
	return idx
}

// snapshot returns a copy of the internal counts under the lock so the
// caller can embed it in Summary without fighting mutation races.
func (e *errCounter) snapshot() ErrorCounts {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.counts
}

// writeSummary marshals s with 2-space indent and writes it to path (or
// stdout if path == ""). The file write is os.OpenFile(O_CREATE|O_TRUNC|
// O_WRONLY, 0644) — the tool is one-off, overwriting prior runs is the
// intended behaviour.
func writeSummary(s Summary, path string) error {
	body, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if path == "" {
		_, err := os.Stdout.Write(append(body, '\n'))
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(body, '\n')); err != nil {
		return err
	}
	return nil
}

// itoa avoids pulling in strconv for the single hot-path call; worker indices
// are bounded by -concurrent so the buffer size is trivial.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
