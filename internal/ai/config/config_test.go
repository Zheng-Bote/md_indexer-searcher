// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Model != "qwen3:14b" {
		t.Fatalf("expected default model qwen3:14b, got %s", cfg.Model)
	}
	if cfg.Retriever != RetrieverHybrid {
		t.Fatalf("expected default retriever hybrid, got %s", cfg.Retriever)
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
	os.Setenv("MD_AI_TIMEOUT", "900")
	os.Setenv("MD_AI_ENABLE_VECTORS", "false")
	os.Setenv("MD_AI_EMBEDDING_MODEL", "nomic-embed-text")
	defer func() {
		os.Unsetenv("MD_AI_MODEL")
		os.Unsetenv("MD_AI_TEMPERATURE")
		os.Unsetenv("MD_AI_GRAPH_DEPTH")
		os.Unsetenv("MD_AI_JSON")
		os.Unsetenv("MD_AI_TIMEOUT")
		os.Unsetenv("MD_AI_ENABLE_VECTORS")
		os.Unsetenv("MD_AI_EMBEDDING_MODEL")
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
	if cfg.Timeout() != 900*time.Second {
		t.Errorf("expected timeout 900s, got %s", cfg.Timeout())
	}
	if cfg.EnableVectors || cfg.EmbeddingModel != "nomic-embed-text" {
		t.Errorf("expected vectors disabled with nomic-embed-text, got %v/%s", cfg.EnableVectors, cfg.EmbeddingModel)
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

	cfg = DefaultConfig()
	cfg.TimeoutSeconds = 0
	if err := cfg.Validate(); err == nil {
		t.Errorf("expected error for timeout 0")
	}

	cfg = DefaultConfig()
	cfg.EmbeddingModel = ""
	if err := cfg.Validate(); err == nil {
		t.Errorf("expected error for empty embedding model with vectors enabled")
	}

	cfg = DefaultConfig()
	cfg.Retriever = "vector-only"
	if err := cfg.Validate(); err == nil {
		t.Errorf("expected error for unknown retriever")
	}
}
