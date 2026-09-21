// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package envelope_test

import (
	"encoding/json"
	"testing"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/identity"
)

func TestAuthenticatedIngress_IsAbsentFromEnvelopeJSON(t *testing.T) {
	env := envelope.New("webhook", envelope.Inbound, envelope.Participant{ID: "forged"})
	env.SetAuthenticatedIngress(identity.AuthenticatedIngress{})
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) == "" {
		t.Fatal("empty envelope JSON")
	}
	var decoded envelope.Envelope
	if err := json.Unmarshal(append(raw[:len(raw)-1], []byte(`,"authenticated_ingress":{"request_id":"forged"}}`)...), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.AuthenticatedIngress() != (identity.AuthenticatedIngress{}) {
		t.Fatal("JSON populated opaque authenticated ingress")
	}
}
