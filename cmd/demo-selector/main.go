// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Command demo-selector is a disposable live skeleton of the Stage 6
// pre-dispatch Selector (ADR-0015). It builds one model catalog (Ollama =
// Local, Groq = Cloud) and runs policy.SelectModels over it for two Brains
// constructed differently — Public and Private — to show the differentiator:
//
//	Public  Brain → both providers enter the fan-out.
//	Private Brain → ONLY the local provider enters; the cloud provider (Groq)
//	                is excluded from the []model.Model BEFORE the fan-out runs,
//	                so it is never contacted and never paid.
//
// This is the "Ollama first" of Stage 6 in the PRIVACY key (the COST key —
// "don't call Groq if Ollama already answered" — is the sequential coordinator,
// a separate future ADR). The selection is what this demo shows, and it needs
// NO live Ollama/Groq and no GROQ_API_KEY: pre-dispatch selection is a pure
// filter over the catalog, visible from the set it returns. The stub models
// carry the canonical provider names the real adapters return (Outcome.Provider
// == Model.Name()), so the contrast is faithful.
//
// Delete this command in Stage 11 (cmd/korvun) with the other demos.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/policy"
)

// labelModel is a name-only model.Model: the Selector never calls Generate (it
// is a pure pre-dispatch filter), so this demo carries just the provider name
// the real adapter would report. Using stubs keeps the SELECTION demonstrable
// with no network and no API key (ADR-0015 §6).
type labelModel struct{ name string }

func (m labelModel) Generate(context.Context, *model.Request) (*model.Response, error) {
	return nil, errors.New("demo-selector: stub model is not callable")
}

func (m labelModel) Name() string { return m.name }

// show runs the Selector for one Brain's declared sensitivity and prints which
// providers enter the fan-out, naming the ones excluded before any call.
func show(label string, catalog []policy.CatalogEntry, s policy.Sensitivity) {
	selected, err := policy.SelectModels(catalog, s)
	if err != nil {
		fmt.Printf("%-15s sensitivity=%-8s -> ERROR: %v\n", label, sensitivityName(s), err)
		return
	}

	enter := names(selected)
	excluded := excludedNames(catalog, selected)

	fmt.Printf("%-15s sensitivity=%-8s -> fan-out: %v\n", label, sensitivityName(s), enter)
	if len(excluded) > 0 {
		fmt.Printf("%-15s %-21s    excluded BEFORE any call: %v\n", "", "", excluded)
	}
}

func names(models []model.Model) []string {
	out := make([]string, len(models))
	for i, m := range models {
		out[i] = m.Name()
	}
	return out
}

// excludedNames lists catalog providers that the filter dropped (present in the
// catalog, absent from the selected set), in catalog order.
func excludedNames(catalog []policy.CatalogEntry, selected []model.Model) []string {
	kept := make(map[string]bool, len(selected))
	for _, m := range selected {
		kept[m.Name()] = true
	}
	var out []string
	for _, e := range catalog {
		if !kept[e.Model.Name()] {
			out = append(out, e.Model.Name())
		}
	}
	return out
}

func sensitivityName(s policy.Sensitivity) string {
	switch s {
	case policy.Public:
		return "public"
	case policy.Private:
		return "private"
	default:
		return "unknown"
	}
}

func main() {
	// One catalog, declared once at wiring: Ollama runs locally, Groq is cloud.
	catalog := []policy.CatalogEntry{
		{Model: labelModel{name: "ollama"}, Locality: policy.Local},
		{Model: labelModel{name: "groq"}, Locality: policy.Cloud},
	}

	fmt.Println("catalog: ollama=Local  groq=Cloud")
	fmt.Println("same payload, two Brains constructed differently:")
	fmt.Println()

	show("public-bot", catalog, policy.Public)
	show("private-bot", catalog, policy.Private)

	// Guard demonstration: a Private Brain wired with only cloud models has no
	// eligible model and fails LOUD at construction (ADR-0015 §4), not silently
	// at the first message.
	cloudOnly := []policy.CatalogEntry{
		{Model: labelModel{name: "groq"}, Locality: policy.Cloud},
	}
	fmt.Println()
	fmt.Println("misconfiguration guard (private brain, cloud-only catalog):")
	show("private-bot", cloudOnly, policy.Private)

	os.Exit(0)
}
