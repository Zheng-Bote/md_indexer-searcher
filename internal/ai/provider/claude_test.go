// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/md-indexer/internal/ai/model"
)

// claudeSSE renders a minimal Messages API stream with the given text deltas and stop data.
func claudeSSE(model string, deltas []string, stopReason, stopDetails string) string {
	var b strings.Builder
	event := func(name, data string) { fmt.Fprintf(&b, "event: %s\ndata: %s\n\n", name, data) }
	event("message_start", fmt.Sprintf(`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":%q,"content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":42,"output_tokens":1}}}`, model))
	event("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	for _, d := range deltas {
		event("content_block_delta", fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%q}}`, d))
	}
	event("content_block_stop", `{"type":"content_block_stop","index":0}`)
	event("message_delta", fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":%q,"stop_sequence":null,"stop_details":%s},"usage":{"output_tokens":7}}`, stopReason, stopDetails))
	event("message_stop", `{"type":"message_stop"}`)
	return b.String()
}

func newClaudeTestServer(t *testing.T, check func(r *http.Request, body map[string]any), status int, sse string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if check != nil {
			check(r, body)
		}
		if status != http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(sse))
	}))
}

func TestClaudeProvider_StreamsAnswerWithEffortAndFallback(t *testing.T) {
	srv := newClaudeTestServer(t, func(r *http.Request, body map[string]any) {
		if r.URL.Path != "/v1/messages" || r.Header.Get("X-Api-Key") != "sk-ant-test" {
			t.Errorf("unexpected request %s key=%q", r.URL.Path, r.Header.Get("X-Api-Key"))
		}
		if !strings.Contains(r.Header.Get("Anthropic-Beta"), "server-side-fallback-2026-07-01") || body["fallbacks"] != "default" {
			t.Errorf("fallback not requested: beta=%q fallbacks=%v", r.Header.Get("Anthropic-Beta"), body["fallbacks"])
		}
		if oc, _ := body["output_config"].(map[string]any); oc["effort"] != "medium" {
			t.Errorf("unexpected output_config %v", body["output_config"])
		}
		if _, ok := body["temperature"]; ok {
			t.Error("temperature must not be sent")
		}
		if body["stream"] != true {
			t.Error("request must stream")
		}
	}, http.StatusOK, claudeSSE("claude-opus-5-5", []string{"Hallo ", "Welt"}, "end_turn", "null"))
	defer srv.Close()

	p := NewClaudeProvider("claude", "sk-ant-test", srv.URL, 10*time.Second, func(string) string { return "medium" }, srv.Client())
	var tokens []string
	res, err := p.GenerateStream(context.Background(), &model.GenerateRequest{Model: "claude-opus-5-5", System: "sys", Prompt: "q", MaxTokens: 1000, Temperature: 0.2}, func(t string) { tokens = append(tokens, t) })
	if err != nil {
		t.Fatal(err)
	}
	if res.Response != "Hallo Welt" || len(tokens) != 2 || res.PromptTokens != 42 || res.OutputTokens != 7 || res.Model != "claude-opus-5-5" {
		t.Errorf("unexpected result %+v", res)
	}
}

func TestClaudeProvider_NoFallbackForHaiku(t *testing.T) {
	srv := newClaudeTestServer(t, func(r *http.Request, body map[string]any) {
		if _, ok := body["fallbacks"]; ok {
			t.Error("fallbacks must not be sent for models without server-side fallback")
		}
	}, http.StatusOK, claudeSSE("claude-haiku-5-5", []string{"ok"}, "end_turn", "null"))
	defer srv.Close()
	p := NewClaudeProvider("claude", "k", srv.URL, 10*time.Second, nil, srv.Client())
	if _, err := p.Generate(context.Background(), &model.GenerateRequest{Model: "claude-haiku-5-5", Prompt: "q", MaxTokens: 100}); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeProvider_ErrorKinds(t *testing.T) {
	cases := []struct {
		name, kind, detail string
		status             int
		sse                string
	}{
		{"refusal", ErrKindRefusal, "cyber", http.StatusOK, claudeSSE("claude-sonnet-5-5", nil, "refusal", `{"type":"refusal","category":"cyber","explanation":"x"}`)},
		{"max_tokens", ErrKindTruncated, "max_tokens", http.StatusOK, claudeSSE("claude-sonnet-5-5", []string{"abgeschn"}, "max_tokens", "null")},
		{"rate limit", ErrKindRateLimit, "", http.StatusTooManyRequests, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newClaudeTestServer(t, nil, tc.status, tc.sse)
			defer srv.Close()
			p := NewClaudeProvider("claude", "k", srv.URL, 10*time.Second, nil, srv.Client())
			_, err := p.Generate(context.Background(), &model.GenerateRequest{Model: "claude-sonnet-5-5", Prompt: "q", MaxTokens: 100})
			var perr *Error
			if !errors.As(err, &perr) || perr.Kind != tc.kind || !strings.Contains(perr.Detail, tc.detail) {
				t.Errorf("expected %s containing %q, got %v", tc.kind, tc.detail, err)
			}
		})
	}
}
