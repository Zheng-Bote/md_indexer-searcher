// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Model != "gemma3:1b" {
		t.Fatalf("expected default model gemma3:1b, got %s", cfg.Model)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should be valid, got: %v", err)
	}
}

func TestConfig_LoadFromEnv(t *testing.T) {
	os.Setenv("MD_AI_MODEL", "llama3")
	os.Setenv("MD_AI_TEMPERATURE", "0.7")
	os.Setenv("MD_AI_GRAPH_DEPTH", "5")
	os.Setenv("MD_AI_JSON", "true")
	defer func() {
		os.Unsetenv("MD_AI_MODEL")
		os.Unsetenv("MD_AI_TEMPERATURE")
		os.Unsetenv("MD_AI_GRAPH_DEPTH")
		os.Unsetenv("MD_AI_JSON")
	}()

	cfg := DefaultConfig()
	cfg.LoadFromEnv()

	if cfg.Model != "llama3" {
		t.Errorf("expected model llama3, got %s", cfg.Model)
	}
	if cfg.Temperature != 0.7 {
		t.Errorf("expected temp 0.7, got %f", cfg.Temperature)
	}
	if cfg.GraphDepth != 5 {
		t.Errorf("expected graph depth 5, got %d", cfg.GraphDepth)
	}
	if !cfg.JSONOutput {
		t.Errorf("expected JSON output true")
	}
}

func TestConfig_Validate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DBPath = ""
	if err := cfg.Validate(); err == nil {
		t.Errorf("expected error for empty db path")
	}

	cfg = DefaultConfig()
	cfg.OllamaHost = ""
	if err := cfg.Validate(); err == nil {
		t.Errorf("expected error for empty host")
	}

	cfg = DefaultConfig()
	cfg.Model = ""
	if err := cfg.Validate(); err == nil {
		t.Errorf("expected error for empty model")
	}

	cfg = DefaultConfig()
	cfg.GraphDepth = 0
	if err := cfg.Validate(); err == nil {
		t.Errorf("expected error for graph depth 0")
	}

	cfg = DefaultConfig()
	cfg.MaxContextTokens = 50
	if err := cfg.Validate(); err == nil {
		t.Errorf("expected error for max context tokens < 100")
	}
}
