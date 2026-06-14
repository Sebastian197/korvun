// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package envelope

// Keyboard is an interactive overlay attached to an Envelope. It is a
// typed 2D arrangement of Buttons (rows of buttons). See ADR-0005.
type Keyboard struct {
	Rows [][]Button `json:"rows"`
}

// Button is a single tap target in a Keyboard. Phase 2E.4 supports
// exactly two button flavours: a callback button (Text + CallbackData)
// and a URL button (Text + URL). Validate enforces that Text is set
// and that exactly one of CallbackData / URL is set. Additional button
// kinds (WebApp, LoginURL, SwitchInlineQuery, CopyText, CallbackGame,
// Pay) are out of scope for this phase and require an amending ADR
// before they appear here.
type Button struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

// CallbackButton returns a Button that triggers a callback query when
// tapped. The data string is what the channel delivers back as the
// CallbackQuery's Data field.
func CallbackButton(text, data string) Button {
	return Button{Text: text, CallbackData: data}
}

// URLButton returns a Button that opens the given URL when tapped.
// The URL must be a valid http(s) URL recognised by the destination
// channel; Validate does not parse it, only that it is non-empty.
func URLButton(text, url string) Button {
	return Button{Text: text, URL: url}
}
