// The cleartext-risk heuristic (ADR-0028 F10). Non-loopback + non-https means
// a pasted bearer would cross the network in the clear. We WARN — we do not
// pretend to block (ADR-0030 §6): the server does not enforce it and a JS
// check is not a security control.
//
// F3 (hardware pass, 2026-07-25): inside Korvun Desktop the builder rides in a
// Wails WebView whose origin is the framework's own custom scheme, NOT https
// and NOT a loopback host — so the plain heuristic false-positived the warning
// even though every byte is in-process/loopback by construction. The Wails
// desktop origins (verified at the wails/v2 source, not memory): macOS/Linux
// serve from `wails://wails/` (darwin frontend.go startURL, scheme "wails");
// the asset-server-port path uses the host `wails.localhost` (RFC-6761
// loopback-reserved). We trust the framework's own `wails:` scheme and that
// reserved `.localhost` host — NOT a bare `http://wails/`, which a LAN host
// named `wails` or spoofed DNS could forge to silence the warning. Recognising
// the genuine desktop origins silences the warning THERE only; a real browser
// (http on a real host) still warns exactly as before.
export function isCleartextRisk(protocol: string, hostname: string): boolean {
  // The Wails desktop WebView — no network hop exists.
  if (protocol === 'wails:' || hostname === 'wails.localhost') return false
  if (protocol === 'https:') return false
  return hostname !== 'localhost' && hostname !== '127.0.0.1' && hostname !== '[::1]'
}

/** cleartextRisk against the live window.location (the App's call site). */
export function cleartextRisk(): boolean {
  return isCleartextRisk(window.location.protocol, window.location.hostname)
}
