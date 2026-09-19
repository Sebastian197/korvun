// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package controlapi

import (
	"net"
	"net/http"

	"github.com/Sebastian197/korvun/internal/action"
)

// peerIsLoopback reports whether the connection's peer address is loopback.
// It judges the TCP peer, nothing else: a process on the same host that
// forwards remote traffic over loopback (a reverse proxy, an SSH tunnel) is a
// loopback peer, and the surface serves it. A RemoteAddr that does not parse
// is not loopback.
func peerIsLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// loopbackOnlyApprovals refuses a non-loopback peer with 403 `loopback_only`
// BEFORE the bearer and before the seam: nothing is read, nothing decided
// (v0.15.1 block B, P2-7; the decision act is signed «loopback, in-process»,
// which is then true of the connection).
func loopbackOnlyApprovals(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !peerIsLoopback(r) {
			writeApprovalError(w, ErrApprovalLoopbackOnly)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// validApprovalIDOrNotFound answers `not_found` for an id no mint produces and
// reports false: the seam is never called with it (v0.15.1 block B, P2-1).
func validApprovalIDOrNotFound(w http.ResponseWriter, id string) bool {
	if !action.ValidApprovalID(id) {
		writeApprovalError(w, ErrApprovalNotFound)
		return false
	}
	return true
}
