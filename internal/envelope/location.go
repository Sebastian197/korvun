// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package envelope

import "encoding/json"

// locationContent is the canonical JSON shape of a Location part's Content
// field, as fixed by ADR-0004. Only lat and lon are part of the contract;
// additional optional companion keys (accuracy, live_period, ...) are
// tolerated on the wire for forward compatibility but not modelled here
// until a future ADR amends the schema.
type locationContent struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// marshalLocation encodes a coordinate pair into the canonical wire form
// for a Location part's Content field.
func marshalLocation(lat, lon float64) string {
	b, _ := json.Marshal(locationContent{Lat: lat, Lon: lon})
	return string(b)
}

// Location decodes the coordinate pair from a Location part's Content
// field. It returns ok=false when the part is not a Location, when its
// Content is empty or not a JSON object, or when either lat or lon is
// missing or not a JSON number.
//
// Unknown extra keys in the JSON object are tolerated and ignored, so
// older binaries can consume Envelopes produced by newer ones; this is
// the forward-compatibility guarantee documented in ADR-0004.
func (p Part) Location() (lat, lon float64, ok bool) {
	if p.Type != Location {
		return 0, 0, false
	}
	if p.Content == "" {
		return 0, 0, false
	}
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(p.Content), &raw); err != nil {
		return 0, 0, false
	}
	latRaw, hasLat := raw["lat"]
	lonRaw, hasLon := raw["lon"]
	if !hasLat || !hasLon {
		return 0, 0, false
	}
	if err := json.Unmarshal(latRaw, &lat); err != nil {
		return 0, 0, false
	}
	if err := json.Unmarshal(lonRaw, &lon); err != nil {
		return 0, 0, false
	}
	return lat, lon, true
}
