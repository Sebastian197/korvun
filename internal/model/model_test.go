// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"context"
	"errors"
	"testing"
)

func TestRole_String(t *testing.T) {
	cases := []struct {
		r    Role
		want string
	}{
		{RoleSystem, "system"},
		{RoleUser, "user"},
		{RoleAssistant, "assistant"},
		{Role(0), "unknown(0)"},
		{Role(99), "unknown(99)"},
	}
	for _, tc := range cases {
		if got := tc.r.String(); got != tc.want {
			t.Errorf("Role(%d).String() = %q, want %q", tc.r, got, tc.want)
		}
	}
}

func TestValidateRequest_rejectsNil(t *testing.T) {
	if err := ValidateRequest(nil); !errors.Is(err, ErrNilRequest) {
		t.Errorf("ValidateRequest(nil) = %v, want ErrNilRequest", err)
	}
}

func TestValidateRequest_rejectsEmptyModel(t *testing.T) {
	req := &Request{
		Messages: []Message{{Role: RoleUser, Content: "hola"}},
	}
	if err := ValidateRequest(req); !errors.Is(err, ErrEmptyModel) {
		t.Errorf("ValidateRequest empty model = %v, want ErrEmptyModel", err)
	}
}

func TestValidateRequest_rejectsEmptyMessages(t *testing.T) {
	req := &Request{Model: "llama3.2"}
	if err := ValidateRequest(req); !errors.Is(err, ErrEmptyMessages) {
		t.Errorf("ValidateRequest empty messages = %v, want ErrEmptyMessages", err)
	}
}

func TestValidateRequest_rejectsInvalidRole(t *testing.T) {
	req := &Request{
		Model:    "llama3.2",
		Messages: []Message{{Role: Role(0), Content: "hola"}},
	}
	if err := ValidateRequest(req); !errors.Is(err, ErrInvalidRole) {
		t.Errorf("ValidateRequest invalid role = %v, want ErrInvalidRole", err)
	}
}

func TestValidateRequest_rejectsEmptyContent(t *testing.T) {
	req := &Request{
		Model:    "llama3.2",
		Messages: []Message{{Role: RoleUser, Content: ""}},
	}
	if err := ValidateRequest(req); !errors.Is(err, ErrEmptyContent) {
		t.Errorf("ValidateRequest empty content = %v, want ErrEmptyContent", err)
	}
}

func TestValidateRequest_acceptsHappyPath(t *testing.T) {
	req := &Request{
		Model: "llama3.2",
		Messages: []Message{
			{Role: RoleSystem, Content: "Eres útil"},
			{Role: RoleUser, Content: "Hola"},
			{Role: RoleAssistant, Content: "Hola, ¿en qué te ayudo?"},
			{Role: RoleUser, Content: "¿Qué hora es?"},
		},
	}
	if err := ValidateRequest(req); err != nil {
		t.Errorf("ValidateRequest happy = %v, want nil", err)
	}
}

// fakeModel is the in-test stand-in used to verify Model is a
// usable interface. It records the last request it received and
// echoes a canned response back; tests of real adapters live in
// their own packages.
type fakeModel struct {
	name     string
	got      *Request
	response *Response
	err      error
}

func (f *fakeModel) Name() string { return f.name }

func (f *fakeModel) Generate(_ context.Context, req *Request) (*Response, error) {
	f.got = req
	if f.err != nil {
		return nil, f.err
	}
	return f.response, nil
}

func TestModel_interfaceContract(t *testing.T) {
	want := &Response{
		Message:   Message{Role: RoleAssistant, Content: "respuesta"},
		Provider:  "fake",
		ModelName: "fake-1",
	}
	var m Model = &fakeModel{name: "fake", response: want}

	if m.Name() != "fake" {
		t.Errorf("Name() = %q, want %q", m.Name(), "fake")
	}

	req := &Request{
		Model:    "fake-1",
		Messages: []Message{{Role: RoleUser, Content: "pregunta"}},
	}
	got, err := m.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate err = %v", err)
	}
	if got != want {
		t.Errorf("Generate = %+v, want %+v", got, want)
	}
}

func TestModel_propagatesError(t *testing.T) {
	sentinel := errors.New("boom")
	var m Model = &fakeModel{name: "fake", err: sentinel}

	req := &Request{
		Model:    "fake-1",
		Messages: []Message{{Role: RoleUser, Content: "x"}},
	}
	_, err := m.Generate(context.Background(), req)
	if !errors.Is(err, sentinel) {
		t.Errorf("Generate err = %v, want sentinel", err)
	}
}
