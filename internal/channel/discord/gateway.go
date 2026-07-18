// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
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
	opReconnect      = 7  // server asks the client to reconnect
	opInvalidSession = 9  // server invalidated the session
	opHello          = 10 // first server frame; carries heartbeat_interval
	opHeartbeatACK   = 11 // server acknowledges a heartbeat
)

// gatewayIntents is the bitfield Korvun subscribes to: GUILDS(1) +
// GUILD_MESSAGES(512) + DIRECT_MESSAGES(4096) + MESSAGE_CONTENT(1<<15). The last is
// a privileged intent, self-serve for bots under 100 servers (ADR-0033 §3).
const gatewayIntents = 1 | 512 | 4096 | (1 << 15) // = 37377

// gatewayReadLimit raises coder/websocket's default 32 KiB per-message cap so a large
// READY payload (many guilds) is not truncated (SetReadLimit, verified via Context7).
const gatewayReadLimit int64 = 1 << 20 // 1 MiB

// reasonBufferSaturated is the drop reason logged when a mapped Envelope cannot be
// enqueued because the inbound buffer is full (backpressure). It is a distinct axis
// from the mapper's dropReason values (which classify why a message was not mapped),
// so it lives here as the single source of truth for the saturation label.
const reasonBufferSaturated = "inbound_buffer_saturated"

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

// heartbeat is a Heartbeat (op 1) frame. D is the last sequence number, or nil
// (JSON null) when none has been received yet.
type heartbeat struct {
	Op int  `json:"op"`
	D  *int `json:"d"`
}

// dial opens the Gateway WebSocket. A dial failure is returned to Receive's caller
// as an honest startup error.
func (a *Adapter) dial(ctx context.Context) (*websocket.Conn, error) {
	conn, _, err := websocket.Dial(ctx, a.cfg.gatewayURL, nil)
	if err != nil {
		return nil, fmt.Errorf("discord: gateway dial: %w", err)
	}
	conn.SetReadLimit(gatewayReadLimit)
	return conn, nil
}

// run drives one Gateway session to completion, then records the terminal cause and
// closes the inbound channel. Closing inbound happens strictly after session has
// joined every gateway goroutine, so a reader that sees inbound closed knows there
// are no leaked goroutines.
func (a *Adapter) run(ctx context.Context, conn *websocket.Conn) {
	defer close(a.inbound)
	err := a.session(ctx, conn)
	a.setTermErr(err)
}

// session performs the handshake and runs the read + heartbeat loops until a
// terminal condition. It returns the named cause, or nil for a clean shutdown
// (ctx cancelled by the caller). ctx is the caller's lifecycle context; loopCtx is
// the internal one both loops share so a zombie/op7/op9 in one unblocks the other.
func (a *Adapter) session(ctx context.Context, conn *websocket.Conn) error {
	loopCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer func() { _ = conn.CloseNow() }()

	interval, err := readHello(loopCtx, conn)
	if err != nil {
		if ctx.Err() != nil { // caller-initiated shutdown mid-handshake is clean, not a fault
			return nil
		}
		return err
	}
	if err := a.sendIdentify(loopCtx, conn); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}

	var (
		writeMu sync.Mutex
		ackOK   atomic.Bool
		lastSeq atomic.Int64 // -1 => no sequence received yet (send JSON null)
	)
	lastSeq.Store(-1)

	var wg sync.WaitGroup
	hbErr := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if e := a.heartbeatLoop(loopCtx, conn, &writeMu, &ackOK, &lastSeq, interval); e != nil {
			select {
			case hbErr <- e:
			default:
			}
			cancel() // wake the read loop so it unwinds
		}
	}()

	readErr := a.readLoop(loopCtx, conn, &writeMu, &ackOK, &lastSeq)
	cancel()  // stop the heartbeat loop
	wg.Wait() // join it before reporting: no goroutine can still be sending

	// A zombie detected by the heartbeat loop is the meaningful cause even though the
	// read loop also unwound with a cancelled-context read error.
	select {
	case e := <-hbErr:
		return e
	default:
	}
	// A caller-initiated cancel is a clean shutdown, not an error.
	if ctx.Err() != nil {
		return nil
	}
	return readErr
}

// readHello reads the first frame, which must be Hello (op 10), and returns the
// heartbeat interval.
func readHello(ctx context.Context, conn *websocket.Conn) (time.Duration, error) {
	var f gatewayPayload
	if err := wsjson.Read(ctx, conn, &f); err != nil {
		return 0, fmt.Errorf("discord: read hello: %w", err)
	}
	if f.Op != opHello {
		return 0, fmt.Errorf("%w: got op %d", ErrUnexpectedFirstFrame, f.Op)
	}
	var hd helloData
	if err := json.Unmarshal(f.D, &hd); err != nil {
		return 0, fmt.Errorf("discord: decode hello: %w", err)
	}
	if hd.HeartbeatInterval <= 0 {
		return 0, fmt.Errorf("%w: heartbeat_interval=%d", ErrUnexpectedFirstFrame, hd.HeartbeatInterval)
	}
	return time.Duration(hd.HeartbeatInterval) * time.Millisecond, nil
}

// sendIdentify reads the bot token from the configured env var AT THIS MOMENT
// (never stored on the Adapter, never logged — ADR-0010) and sends the Identify.
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

// readLoop consumes Gateway frames until a terminal condition. It tracks the
// sequence number, captures selfID + session info from READY, maps MESSAGE_CREATE
// dispatches, records heartbeat ACKs, answers server-initiated heartbeat requests,
// and turns op7/op9 into named errors.
func (a *Adapter) readLoop(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, ackOK *atomic.Bool, lastSeq *atomic.Int64) error {
	var selfID string
	for {
		var f gatewayPayload
		if err := wsjson.Read(ctx, conn, &f); err != nil {
			return fmt.Errorf("discord: gateway read: %w", err)
		}
		if f.S != nil {
			lastSeq.Store(int64(*f.S))
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
					selfID = rd.User.ID
					a.ready.Store(&readySession{id: rd.SessionID, resumeURL: rd.ResumeGatewayURL})
					a.cfg.logger.InfoContext(ctx, "discord: gateway ready",
						"channel", ChannelName, "session_id", rd.SessionID)
				}
			case "MESSAGE_CREATE":
				a.handleMessage(ctx, f.D, selfID)
			}
		case opHeartbeat:
			// Server asked for an immediate heartbeat.
			if err := sendHeartbeat(ctx, conn, writeMu, lastSeq); err != nil {
				return err
			}
		case opHeartbeatACK:
			ackOK.Store(true)
		case opReconnect:
			return ErrGatewayReconnect
		case opInvalidSession:
			return ErrGatewayInvalidSession
		}
	}
}

// handleMessage maps a MESSAGE_CREATE dispatch and enqueues it, or counts and logs
// the drop with its reason.
func (a *Adapter) handleMessage(ctx context.Context, data json.RawMessage, selfID string) {
	env, reason := mapMessageCreate(data, selfID)
	if reason != keep {
		a.dropped.Add(1)
		a.cfg.logger.WarnContext(ctx, "discord: dropped inbound message",
			"channel", ChannelName, "reason", reason.String())
		return
	}
	// Non-blocking enqueue: a saturated buffer drops at the edge (backpressure) with
	// its own reason rather than blocking the read loop.
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
// time the next one is due, it returns ErrZombieConnection.
func (a *Adapter) heartbeatLoop(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, ackOK *atomic.Bool, lastSeq *atomic.Int64, interval time.Duration) error {
	jitter := a.cfg.jitterFrac()
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
		// A cancelled ctx (caller shutdown, or op7/op9 propagated from the read loop
		// via cancel) can race a ready timer.C in the select above; re-check here so a
		// teardown is never misreported as a zombie.
		if ctx.Err() != nil {
			return nil
		}
		if awaitingACK && !ackOK.Load() {
			return ErrZombieConnection
		}
		ackOK.Store(false)
		awaitingACK = true
		if err := sendHeartbeat(ctx, conn, writeMu, lastSeq); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		timer.Reset(interval)
	}
}

// sendHeartbeat writes an op-1 frame carrying the last sequence number (or null),
// serialising with the shared write mutex so it never races the Identify or a
// server-requested beat.
func sendHeartbeat(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, lastSeq *atomic.Int64) error {
	writeMu.Lock()
	defer writeMu.Unlock()

	frame := heartbeat{Op: opHeartbeat}
	if s := lastSeq.Load(); s >= 0 {
		v := int(s)
		frame.D = &v
	}
	if err := wsjson.Write(ctx, conn, frame); err != nil {
		return fmt.Errorf("discord: send heartbeat: %w", err)
	}
	return nil
}
