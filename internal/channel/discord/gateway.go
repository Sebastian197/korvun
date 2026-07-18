// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Gateway opcodes (ADR-0033 §3 / Discord Gateway spec, verified via Context7).
const (
	opDispatch       = 0  // an event (`t` names it); carries a sequence number `s`
	opHeartbeat      = 1  // client -> server heartbeat, or server -> client "beat now"
	opIdentify       = 2  // client -> server initial handshake
	opResume         = 6  // client -> server resume of a dropped session
	opReconnect      = 7  // server asks the client to reconnect
	opInvalidSession = 9  // server invalidated the session (d = resumable bool)
	opHello          = 10 // first server frame; carries heartbeat_interval
	opHeartbeatACK   = 11 // server acknowledges a heartbeat
)

// gatewayIntents is the bitfield Korvun subscribes to: GUILDS(1) +
// GUILD_MESSAGES(512) + DIRECT_MESSAGES(4096) + MESSAGE_CONTENT(1<<15). The last is a
// privileged intent; per Discord's 2026-06-11 policy it is self-serve for bots under
// 10,000 USERS (the separate 100-server bot-verification process gates OTHER
// capabilities, not this intent toggle). Every Korvun user runs their own bot, so it
// stays below the threshold by construction (ADR-0033 §3).
const gatewayIntents = 1 | 512 | 4096 | (1 << 15) // = 37377

// gatewayReadLimit raises coder/websocket's default 32 KiB per-message cap so a large
// READY payload (many guilds) is not truncated (SetReadLimit, verified via Context7).
const gatewayReadLimit int64 = 1 << 20 // 1 MiB

// reasonBufferSaturated is the drop reason logged when a mapped Envelope cannot be
// enqueued because the inbound buffer is full (backpressure). Distinct axis from the
// mapper's dropReason values; the single source of truth for the saturation label.
const reasonBufferSaturated = "inbound_buffer_saturated"

// Reconnect backoff bounds (full jitter, drawn in [0, min(cap, base·2ⁿ)) — the retry
// decorator's grammar, ADR-0031).
const (
	reconnectBackoffBase = 1 * time.Second
	reconnectBackoffCap  = 60 * time.Second
)

// op9ReidentifyWait bounds the random wait before a fresh Identify after an Invalid
// Session (op 9, d=false): Discord mandates a uniform draw in [1s, 5s) — verified via
// Context7 — independent of the exponential reconnect backoff.
const (
	op9ReidentifyWaitBase = 1 * time.Second
	op9ReidentifyWaitSpan = 4 * time.Second
)

// minStableUptime is how long a connected session must last before its drop resets
// the backoff. A handshake-then-instant-drop flap stays "unstable", so exponential
// backoff engages and the supervisor never storms the Gateway with zero-wait redials.
const minStableUptime = 10 * time.Second

// gatewayPayload is the Gateway frame envelope: op is the opcode, d the event data
// (raw so each op decodes its own shape), s the sequence number (nil when absent),
// and t the event name for op-0 dispatches.
type gatewayPayload struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d,omitempty"`
	S  *int            `json:"s,omitempty"`
	T  string          `json:"t,omitempty"`
}

// helloData is the `d` of a Hello (op 10).
type helloData struct {
	HeartbeatInterval int `json:"heartbeat_interval"` // milliseconds
}

// readyData is the reconnect-relevant subset of a READY event's `d`.
type readyData struct {
	SessionID        string `json:"session_id"`
	ResumeGatewayURL string `json:"resume_gateway_url"`
	User             struct {
		ID string `json:"id"`
	} `json:"user"`
}

// identify is an Identify (op 2) frame.
type identify struct {
	Op int          `json:"op"`
	D  identifyData `json:"d"`
}

// identifyData is the `d` of an Identify: the bot token (read from env at connect
// time, never stored), the intents bitfield, and the connection properties.
type identifyData struct {
	Token      string        `json:"token"`
	Intents    int           `json:"intents"`
	Properties identifyProps `json:"properties"`
}

// identifyProps are the Identify connection properties Discord expects.
type identifyProps struct {
	OS      string `json:"os"`
	Browser string `json:"browser"`
	Device  string `json:"device"`
}

// resume is a Resume (op 6) frame: it replays events from `seq` for `session_id`
// without a fresh Identify (verified via Context7).
type resume struct {
	Op int        `json:"op"`
	D  resumeData `json:"d"`
}

// resumeData is the `d` of a Resume.
type resumeData struct {
	Token     string `json:"token"`
	SessionID string `json:"session_id"`
	Seq       int    `json:"seq"`
}

// heartbeat is a Heartbeat (op 1) frame. D is the last sequence number, or nil (JSON
// null) when none has been received yet.
type heartbeat struct {
	Op int  `json:"op"`
	D  *int `json:"d"`
}

// sessionState is the reconnect-relevant state that PERSISTS across sessions, owned
// by the supervisor. selfID (the bot's own id) survives every resume so loop
// prevention keeps working; session_id + resume URL + lastSeq drive a resume.
type sessionState struct {
	sessionID string
	resumeURL string
	selfID    string
	lastSeq   atomic.Int64 // -1 = no sequence received yet (concurrent: read loop + heartbeat)
}

func (s *sessionState) reset() {
	s.sessionID = ""
	s.resumeURL = ""
	s.selfID = ""
	s.lastSeq.Store(-1)
}

// connectStrategy is how the next connection begins: a fresh Identify or a Resume.
type connectStrategy int

const (
	connectIdentify connectStrategy = iota
	connectResume
)

func (s connectStrategy) String() string {
	if s == connectResume {
		return "resume"
	}
	return "identify"
}

// outcomeKind classifies how a session ended, and thus what the supervisor does next.
type outcomeKind int

const (
	outcomeCleanStop  outcomeKind = iota // caller ctx cancelled — stop, close inbound
	outcomeFatal                         // non-recoverable — stop, surface the cause
	outcomeResume                        // reconnect and Resume
	outcomeReidentify                    // reconnect and re-Identify (fresh session)
)

// sessionOutcome is a session's terminal classification. connected reports whether
// the session reached READY/RESUMED (used to reset the backoff).
type sessionOutcome struct {
	kind      outcomeKind
	err       error
	connected bool
}

// supervise is the reconnect loop that owns the inbound channel. It keeps a session
// alive for as long as ctx lives: dialing, identifying, resuming after a drop,
// re-identifying when a resume is rejected, each attempt separated by exponential
// backoff. inbound is closed exactly once — on a clean ctx-cancel stop or a fatal
// cause — never on a reconnect. See the decision map in the SP4 commit.
func (a *Adapter) supervise(ctx context.Context) {
	defer close(a.inbound)

	st := &sessionState{}
	st.lastSeq.Store(-1)
	strategy := connectIdentify
	attempt := 0           // consecutive failures without a stable connect (backoff index+1)
	var wait time.Duration // wait before the next dial (0 for the first)

	for {
		if ctx.Err() != nil {
			a.setTermErr(nil)
			return
		}
		if wait > 0 {
			if err := a.cfg.clock.Sleep(ctx, wait); err != nil {
				a.setTermErr(nil) // ctx cancelled during the wait = clean stop
				return
			}
		}

		dialURL := a.cfg.gatewayURL
		if strategy == connectResume && st.resumeURL != "" {
			dialURL = withGatewayQuery(st.resumeURL)
		}
		conn, err := a.dial(ctx, dialURL)
		if err != nil {
			if ctx.Err() != nil {
				a.setTermErr(nil)
				return
			}
			attempt++
			wait = a.backoff(attempt - 1)
			a.reconnects.Add(1)
			a.cfg.logger.WarnContext(ctx, "discord: gateway dial failed, will retry",
				"channel", ChannelName, "strategy", strategy.String(), "attempt", attempt,
				"wait", wait.String(), "error", err.Error())
			continue
		}

		start := time.Now()
		outcome := a.runSession(ctx, conn, st, strategy)
		switch outcome.kind {
		case outcomeCleanStop:
			a.setTermErr(nil)
			return
		case outcomeFatal:
			a.setTermErr(outcome.err)
			a.cfg.logger.ErrorContext(ctx, "discord: gateway stopped on a fatal cause (no reconnect)",
				"channel", ChannelName, "error", outcome.err.Error())
			return
		case outcomeResume:
			strategy = connectResume
		case outcomeReidentify:
			strategy = connectIdentify
			st.reset()
		}

		// Only a session that stayed connected LONG ENOUGH resets the backoff; a
		// handshake-then-instant-drop flap stays unstable so backoff engages (no storm).
		stable := outcome.connected && time.Since(start) >= minStableUptime
		if stable {
			attempt = 0
		} else {
			attempt++
		}

		switch {
		case errors.Is(outcome.err, errInvalidSessionFresh):
			wait = a.op9ReidentifyWait() // Discord's mandated 1-5s pre-Identify wait
		case stable:
			wait = 0 // a healthy session that dropped (e.g. op7) reconnects promptly
		default:
			wait = a.backoff(attempt - 1)
		}

		a.reconnects.Add(1)
		a.cfg.logger.WarnContext(ctx, "discord: gateway session ended, reconnecting",
			"channel", ChannelName, "strategy", strategy.String(), "attempt", attempt,
			"wait", wait.String(), "cause", errString(outcome.err))
	}
}

// backoff returns the full-jitter wait for a 0-based attempt index: a uniform draw in
// [0, min(cap, base·2ⁿ)).
func (a *Adapter) backoff(attempt int) time.Duration {
	step := reconnectBackoffBase << attempt
	if step <= 0 || step > reconnectBackoffCap { // clamp (and guard shift overflow)
		step = reconnectBackoffCap
	}
	return time.Duration(a.cfg.rnd() * float64(step))
}

// op9ReidentifyWait draws the random 1-5s wait Discord mandates before a fresh
// Identify following an Invalid Session (op 9, d=false).
func (a *Adapter) op9ReidentifyWait() time.Duration {
	return op9ReidentifyWaitBase + time.Duration(a.cfg.rnd()*float64(op9ReidentifyWaitSpan))
}

// dial opens the Gateway WebSocket at url and raises the read limit.
func (a *Adapter) dial(ctx context.Context, url string) (*websocket.Conn, error) {
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, fmt.Errorf("discord: gateway dial: %w", err)
	}
	conn.SetReadLimit(gatewayReadLimit)
	return conn, nil
}

// runSession performs one connection's handshake (Hello -> Identify or Resume) and
// runs the read + heartbeat loops until a terminal condition, returning the classified
// outcome. It never closes the inbound channel (the supervisor owns it).
func (a *Adapter) runSession(ctx context.Context, conn *websocket.Conn, st *sessionState, strategy connectStrategy) sessionOutcome {
	loopCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer func() { _ = conn.CloseNow() }()

	interval, err := readHello(loopCtx, conn)
	if err != nil {
		return a.classifyEnd(ctx, st, false, err)
	}
	switch strategy {
	case connectResume:
		if err := a.sendResume(loopCtx, conn, st); err != nil {
			return a.classifyEnd(ctx, st, false, err)
		}
	default:
		if err := a.sendIdentify(loopCtx, conn); err != nil {
			return a.classifyEnd(ctx, st, false, err)
		}
	}

	var (
		writeMu sync.Mutex
		ackOK   atomic.Bool
		wg      sync.WaitGroup
	)
	hbErr := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if e := a.heartbeatLoop(loopCtx, conn, &writeMu, &ackOK, st, interval); e != nil {
			select {
			case hbErr <- e:
			default:
			}
			cancel() // wake the read loop
		}
	}()

	connected, readErr := a.readLoop(loopCtx, conn, &writeMu, &ackOK, st)
	cancel()
	wg.Wait()

	// A zombie detected by the heartbeat loop is the meaningful cause even though the
	// read loop also unwound with a cancelled-context read error.
	select {
	case e := <-hbErr:
		return a.classifyEnd(ctx, st, connected, e)
	default:
	}
	return a.classifyEnd(ctx, st, connected, readErr)
}

// classifyEnd maps a session's terminating cause to a supervisor action (the R1..R4
// discipline): clean stop on a caller cancel; fatal (no retry) on a missing token or a
// non-recoverable close code; resume on op7 / zombie / op9-resumable / a drop that
// still has a session; re-identify on op9-non-resumable or a drop with no session.
func (a *Adapter) classifyEnd(ctx context.Context, st *sessionState, connected bool, cause error) sessionOutcome {
	if ctx.Err() != nil {
		return sessionOutcome{kind: outcomeCleanStop, connected: connected}
	}
	switch {
	case errors.Is(cause, ErrMissingToken):
		return sessionOutcome{kind: outcomeFatal, err: cause, connected: connected}
	case errors.Is(cause, errReconnectOp7),
		errors.Is(cause, errZombie),
		errors.Is(cause, errInvalidSessionResumable):
		return sessionOutcome{kind: outcomeResume, err: cause, connected: connected}
	case errors.Is(cause, errInvalidSessionFresh):
		return sessionOutcome{kind: outcomeReidentify, err: cause, connected: connected}
	}
	if code := websocket.CloseStatus(cause); code != -1 && isFatalCloseCode(int(code)) {
		return sessionOutcome{
			kind:      outcomeFatal,
			err:       fmt.Errorf("%w (code %d)", ErrGatewayFatalClose, int(code)),
			connected: connected,
		}
	}
	// Network drop / no close code / reconnectable close code: resume if a session
	// exists to resume, else start fresh.
	if st.sessionID != "" {
		return sessionOutcome{kind: outcomeResume, err: cause, connected: connected}
	}
	return sessionOutcome{kind: outcomeReidentify, err: cause, connected: connected}
}

// isFatalCloseCode reports the Gateway close codes that must NOT be retried:
// authentication failed (4004), invalid shard (4010), sharding required (4011),
// invalid API version (4012), invalid intents (4013), disallowed intents (4014).
// Verified via Context7.
func isFatalCloseCode(code int) bool {
	switch code {
	case 4004, 4010, 4011, 4012, 4013, 4014:
		return true
	default:
		return false
	}
}

// withGatewayQuery appends the required v10/JSON query to a bare resume_gateway_url
// (Discord returns it without query params; the resume connection reuses the initial
// connection's params).
func withGatewayQuery(raw string) string {
	if strings.Contains(raw, "?") {
		return raw
	}
	return strings.TrimSuffix(raw, "/") + "/?v=10&encoding=json"
}

// readHello reads the first frame, which must be Hello (op 10), and returns the
// heartbeat interval.
func readHello(ctx context.Context, conn *websocket.Conn) (time.Duration, error) {
	var f gatewayPayload
	if err := wsjson.Read(ctx, conn, &f); err != nil {
		return 0, fmt.Errorf("discord: read hello: %w", err)
	}
	if f.Op != opHello {
		return 0, fmt.Errorf("%w: got op %d", errUnexpectedFirstFrame, f.Op)
	}
	var hd helloData
	if err := json.Unmarshal(f.D, &hd); err != nil {
		return 0, fmt.Errorf("%w: decode: %v", errUnexpectedFirstFrame, err)
	}
	if hd.HeartbeatInterval <= 0 {
		return 0, fmt.Errorf("%w: heartbeat_interval=%d", errUnexpectedFirstFrame, hd.HeartbeatInterval)
	}
	return time.Duration(hd.HeartbeatInterval) * time.Millisecond, nil
}

// sendIdentify reads the bot token from the configured env var AT THIS MOMENT (never
// stored on the Adapter, never logged — ADR-0010) and sends the Identify.
func (a *Adapter) sendIdentify(ctx context.Context, conn *websocket.Conn) error {
	token := os.Getenv(a.cfg.tokenEnv)
	if token == "" {
		return fmt.Errorf("%w: %q (discord bot token)", ErrMissingToken, a.cfg.tokenEnv)
	}
	frame := identify{
		Op: opIdentify,
		D: identifyData{
			Token:   token,
			Intents: gatewayIntents,
			Properties: identifyProps{
				OS:      runtime.GOOS,
				Browser: "korvun",
				Device:  "korvun",
			},
		},
	}
	if err := wsjson.Write(ctx, conn, frame); err != nil {
		return fmt.Errorf("discord: send identify: %w", err)
	}
	return nil
}

// sendResume replays a dropped session: it reads the token env-only (ADR-0010) and
// sends Resume (op 6) with the stored session_id + last sequence number.
func (a *Adapter) sendResume(ctx context.Context, conn *websocket.Conn, st *sessionState) error {
	token := os.Getenv(a.cfg.tokenEnv)
	if token == "" {
		return fmt.Errorf("%w: %q (discord bot token)", ErrMissingToken, a.cfg.tokenEnv)
	}
	seq := st.lastSeq.Load()
	if seq < 0 {
		seq = 0
	}
	frame := resume{Op: opResume, D: resumeData{Token: token, SessionID: st.sessionID, Seq: int(seq)}}
	if err := wsjson.Write(ctx, conn, frame); err != nil {
		return fmt.Errorf("discord: send resume: %w", err)
	}
	return nil
}

// readLoop consumes Gateway frames until a terminal condition. It tracks the sequence
// number, captures/refreshes the session state from READY, marks a resume complete on
// RESUMED, maps MESSAGE_CREATE dispatches, records heartbeat ACKs, answers
// server-initiated heartbeat requests, and turns op7/op9 into typed signals. It
// returns whether the session reached a connected state and the terminating cause.
func (a *Adapter) readLoop(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, ackOK *atomic.Bool, st *sessionState) (bool, error) {
	connected := false
	for {
		var f gatewayPayload
		if err := wsjson.Read(ctx, conn, &f); err != nil {
			return connected, err
		}
		if f.S != nil {
			st.lastSeq.Store(int64(*f.S))
		}
		switch f.Op {
		case opDispatch:
			switch f.T {
			case "READY":
				var rd readyData
				if err := json.Unmarshal(f.D, &rd); err != nil {
					a.cfg.logger.WarnContext(ctx, "discord: could not decode READY payload",
						"channel", ChannelName, "error", err.Error())
				} else {
					st.selfID = rd.User.ID
					st.sessionID = rd.SessionID
					st.resumeURL = rd.ResumeGatewayURL
					a.ready.Store(&readySession{id: rd.SessionID, resumeURL: rd.ResumeGatewayURL})
					connected = true
					a.cfg.logger.InfoContext(ctx, "discord: gateway ready",
						"channel", ChannelName, "session_id", rd.SessionID)
				}
			case "RESUMED":
				connected = true
				a.cfg.logger.InfoContext(ctx, "discord: gateway resumed",
					"channel", ChannelName, "session_id", st.sessionID)
			case "MESSAGE_CREATE":
				a.handleMessage(ctx, f.D, st.selfID)
			}
		case opHeartbeat:
			if err := sendHeartbeat(ctx, conn, writeMu, st); err != nil {
				return connected, err
			}
		case opHeartbeatACK:
			ackOK.Store(true)
		case opReconnect:
			return connected, errReconnectOp7
		case opInvalidSession:
			var resumable bool
			_ = json.Unmarshal(f.D, &resumable)
			if resumable {
				return connected, errInvalidSessionResumable
			}
			return connected, errInvalidSessionFresh
		}
	}
}

// handleMessage maps a MESSAGE_CREATE dispatch and enqueues it, or counts and logs the
// drop with its reason.
func (a *Adapter) handleMessage(ctx context.Context, data json.RawMessage, selfID string) {
	env, reason := mapMessageCreate(data, selfID)
	if reason != keep {
		a.dropped.Add(1)
		a.cfg.logger.WarnContext(ctx, "discord: dropped inbound message",
			"channel", ChannelName, "reason", reason.String())
		return
	}
	select {
	case a.inbound <- env:
		return
	default:
	}
	a.dropped.Add(1)
	a.cfg.logger.WarnContext(ctx, "discord: dropped inbound message",
		"channel", ChannelName, "reason", reasonBufferSaturated, "envelope_id", env.ID)
}

// heartbeatLoop sends a heartbeat every interval (after the initial startup jitter)
// and detects a zombie connection: if the previous heartbeat was never ACKed by the
// time the next is due, it returns errZombie.
func (a *Adapter) heartbeatLoop(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, ackOK *atomic.Bool, st *sessionState, interval time.Duration) error {
	jitter := a.cfg.rnd()
	if jitter < 0 || jitter >= 1 {
		jitter = 0
	}
	timer := time.NewTimer(time.Duration(float64(interval) * jitter))
	defer timer.Stop()

	awaitingACK := false
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
		}
		if ctx.Err() != nil {
			return nil
		}
		if awaitingACK && !ackOK.Load() {
			return errZombie
		}
		ackOK.Store(false)
		awaitingACK = true
		if err := sendHeartbeat(ctx, conn, writeMu, st); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		timer.Reset(interval)
	}
}

// sendHeartbeat writes an op-1 frame carrying the last sequence number (or null),
// serialising with the shared write mutex so it never races the handshake or a
// server-requested beat.
func sendHeartbeat(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, st *sessionState) error {
	writeMu.Lock()
	defer writeMu.Unlock()

	frame := heartbeat{Op: opHeartbeat}
	if s := st.lastSeq.Load(); s >= 0 {
		v := int(s)
		frame.D = &v
	}
	if err := wsjson.Write(ctx, conn, frame); err != nil {
		return fmt.Errorf("discord: send heartbeat: %w", err)
	}
	return nil
}

// errString renders an error for a structured log field, empty for nil.
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
