// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The v0.15.1 block A moulds name the delivery sentinels through these two
// variables. During the red phase they stood in for sentinels that did not
// exist yet; the green phase bound them to the production sentinels, and they
// stay as the moulds' one handle on them.
//
// The two text constants are the EXACT sentences the sentinels must carry, so
// a rewording of either — for instance a «not sent» that starts claiming
// delivery — reddens the moulds that pin them.
package tool

var (
	errDeliveryUnknown = ErrDeliveryUnknown
	errNotSent         = ErrNotSent
)

// deliveryUnknownText is the EXACT text ErrDeliveryUnknown must carry: a
// connection was obtained and no answer was read, and the sentence claims
// neither that the request left nor that it did not.
const deliveryUnknownText = "tool: a connection was obtained and no answer was read; whether the request left is not known"

// notSentText is the EXACT text ErrNotSent must carry.
const notSentText = "tool: no connection was obtained; nothing was sent"
