// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package conversation

import (
	"fmt"
	"strings"
	"sync"
)

// Message represents a single chat turn.
type Message struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

// Session manages conversation history for multi-turn interactions.
type Session struct {
	mu       sync.RWMutex
	messages []Message
}

// NewSession creates an empty conversation session.
func NewSession() *Session {
	return &Session{
		messages: make([]Message, 0),
	}
}

// AddTurn appends user question and assistant answer to the session.
func (s *Session) AddTurn(userQuery, assistantAnswer string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages,
		Message{Role: "user", Content: userQuery},
		Message{Role: "assistant", Content: assistantAnswer},
	)
}

// Messages returns a copy of history.
func (s *Session) Messages() []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]Message, len(s.messages))
	copy(res, s.messages)
	return res
}

// FormatHistory formats prior turns as a string for conversation context.
func (s *Session) FormatHistory() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.messages) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, m := range s.messages {
		roleName := strings.ToUpper(m.Role[:1]) + m.Role[1:]
		sb.WriteString(fmt.Sprintf("%s: %s\n", roleName, m.Content))
	}
	return sb.String()
}

// Clear resets the session history.
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = make([]Message, 0)
}
