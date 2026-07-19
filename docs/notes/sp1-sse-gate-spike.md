# SP1 — SSE flush gate spike (evidence record)

> **Purpose:** auditable record of the spike that resolved the ADR-0035 §3(b)
> gate (AssetServer proxy vs restricted CORS). Run 2026-07-19 on Chano's iMac
> (Intel, macOS 13, WKWebView), Wails `v2.13.0`, Go 1.26.5. The spike itself
> was a throwaway module outside the repo; this note preserves the
> methodology and numbers.

## Question

Does the Wails v2 AssetServer `Handler` deliver **flushed** SSE responses to
the WebView **incrementally**, or buffered? A buffering path would silently
break the live-view (`GET /api/events`) through the planned same-origin proxy
and force the restricted-CORS fallback.

## Method

A minimal real Wails app (window opened on real hardware, not an httptest):

- **Backend:** `assetserver.Options.Handler` serving `/sse` — 3 events, 400 ms
  apart, `http.Flusher.Flush()` after each, server-side send offsets logged.
- **Frontend:** `EventSource('/sse')` recording per-event arrival offsets
  (`performance.now()`), then a second pass with `fetch` + `ReadableStream`
  reader recording per-chunk arrival; results returned through a Wails
  binding and persisted to disk; 25 s watchdog for the hang case.

## Result (raw)

```json
{"es":[34,419,820],"fetch":[11,423,823],"serverSendMs":[0,402,803,0,419,820]}
```

Server flushed at 0/402/803 ms; EventSource events arrived at 34/419/820 ms;
the fetch streaming reader saw chunks at 11/423/823 ms. Every event arrived
within ~20–35 ms of its flush. No `esError`, no timeout — EventSource works
over the Wails asset scheme on WKWebView.

## Verdict

**Streams incrementally → the ADR-0035 §3(b) proxy path stands; the CORS
fallback is not needed.** Scope honesty: validated on macOS/WKWebView (the
hardware-validation platform); Windows (WebView2) and Linux (WebKitGTK) get a
re-check when their packaging lanes first run.

## Environment finding (recorded for the build lane)

On macOS 13 (older Xcode CLT) linking any Wails v2 binary needs the
UniformTypeIdentifiers framework made explicit or `UTType` is an undefined
symbol:

```sh
CGO_LDFLAGS="-framework UniformTypeIdentifiers" go build -tags desktop,production ./cmd/korvun-desktop
```
