// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/md-indexer/internal/ai/model"
)

func TestOpenAICompatProvider_StreamsWithUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer or-key" {
			t.Errorf("unexpected request %s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["stream"] != true || body["stream_options"] == nil || len(body["messages"].([]any)) != 2 {
			t.Errorf("unexpected body %v", body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"model\":\"deepseek/deepseek-r1:free\",\"choices\":[{\"delta\":{\"content\":\"Hallo \"}}]}\n\n" +
			": keep-alive\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"Welt\"},\"finish_reason\":\"stop\"}]}\n\n" +
			"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":40,\"completion_tokens\":2}}\n\n" +
			"data: [DONE]\n\n"))
	}))
	defer srv.Close()

	// The endpoint may be configured with the full path.
	p := NewOpenAICompatProvider("openrouter", srv.URL+"/v1/chat/completions/", "or-key", srv.Client())
	var tokens []string
	res, err := p.GenerateStream(context.Background(), &model.GenerateRequest{Model: "deepseek/deepseek-r1:free", System: "s", Prompt: "q", MaxTokens: 100}, func(t string) { tokens = append(tokens, t) })
	if err != nil {
		t.Fatal(err)
	}
	if res.Response != "Hallo Welt" || len(tokens) != 2 || res.PromptTokens != 40 || res.OutputTokens != 2 || res.Model != "deepseek/deepseek-r1:free" {
		t.Errorf("unexpected result %+v", res)
	}
}

func TestOpenAICompatProvider_Errors(t *testing.T) {
	cases := map[string]func(w http.ResponseWriter){
		ErrKindAuth:      func(w http.ResponseWriter) { w.WriteHeader(401) },
		ErrKindRateLimit: func(w http.ResponseWriter) { w.WriteHeader(429) },
		ErrKindUnavailable: func(w http.ResponseWriter) {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":{"message":"model not found"}}`))
		},
		ErrKindTruncated: func(w http.ResponseWriter) {
			w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"abgeschn\"},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n"))
		},
	}
	for kind, handler := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler(w) }))
		_, err := NewOpenAICompatProvider("gemini", srv.URL, "k", srv.Client()).Generate(context.Background(), &model.GenerateRequest{Model: "m"})
		srv.Close()
		var perr *Error
		if !errors.As(err, &perr) || perr.Kind != kind || perr.Provider != "gemini" {
			t.Errorf("expected %s, got %v", kind, err)
		}
	}
}

func TestStatusError_GeminiInvalidKeyIsAuth(t *testing.T) {
	err := statusError("gemini", 400, `[{"error":{"code":400,"message":"Please pass a valid API key","status":"INVALID_ARGUMENT"}}]`)
	if err.Kind != ErrKindAuth {
		t.Errorf("expected auth, got %s", err.Kind)
	}
}
