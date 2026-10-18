// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Error kinds reported to callers so they can show a specific message.
const (
	ErrKindAuth        = "auth"
	ErrKindRateLimit   = "rate_limit"
	ErrKindRefusal     = "refusal"
	ErrKindTruncated   = "truncated"
	ErrKindTimeout     = "timeout"
	ErrKindUnavailable = "unavailable"
)

// Error is a provider failure with a classified kind. It never contains API keys.
type Error struct {
	Provider string
	Kind     string
	Status   int
	Detail   string
}

func (e *Error) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("%s: %s (HTTP %d): %s", e.Provider, e.Kind, e.Status, e.Detail)
	}
	return fmt.Sprintf("%s: %s: %s", e.Provider, e.Kind, e.Detail)
}

// statusError classifies a non-2xx HTTP response.
func statusError(provider string, status int, body string) *Error {
	kind := ErrKindUnavailable
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		kind = ErrKindAuth
	case http.StatusTooManyRequests:
		kind = ErrKindRateLimit
	case http.StatusBadRequest:
		// Gemini answers an invalid key with 400 INVALID_ARGUMENT ("API_KEY_INVALID" / "valid API key").
		if lower := strings.ToLower(body); strings.Contains(lower, "api_key_invalid") || strings.Contains(lower, "valid api key") {
			kind = ErrKindAuth
		}
	}
	if len(body) > 300 {
		body = body[:300]
	}
	return &Error{Provider: provider, Kind: kind, Status: status, Detail: body}
}

// transportError classifies a failed HTTP call (timeout or network).
func transportError(provider string, err error) *Error {
	kind := ErrKindUnavailable
	if errors.Is(err, context.DeadlineExceeded) {
		kind = ErrKindTimeout
	}
	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) && netErr.Timeout() {
		kind = ErrKindTimeout
	}
	return &Error{Provider: provider, Kind: kind, Detail: err.Error()}
}
