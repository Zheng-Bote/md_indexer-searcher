// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config encapsulates configuration parameters for md-ai.
type Config struct {
	DBPath           string  `json:"db_path" yaml:"db_path"`
	OllamaHost       string  `json:"ollama_host" yaml:"ollama_host"`
	Model            string  `json:"model" yaml:"model"`
	Temperature      float64 `json:"temperature" yaml:"temperature"`
	MaxTokens        int     `json:"max_tokens" yaml:"max_tokens"`
	MaxContextTokens int     `json:"max_context_tokens" yaml:"max_context_tokens"`
	GraphDepth       int     `json:"graph_depth" yaml:"graph_depth"`
	TimeoutSeconds   int     `json:"timeout_seconds" yaml:"timeout_seconds"`
	JSONOutput       bool    `json:"json_output" yaml:"json_output"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		DBPath:           "knowledge.db",
		OllamaHost:       "http://localhost:11434",
		Model:            "gemma3:1b",
		Temperature:      0.2,
		MaxTokens:        2048,
		MaxContextTokens: 16000,
		GraphDepth:       3,
		TimeoutSeconds:   600,
		JSONOutput:       false,
	}
}

// LoadFromEnv overrides configuration values with environment variables if present.
func (c *Config) LoadFromEnv() {
	if val := os.Getenv("MD_AI_DB_PATH"); val != "" {
		c.DBPath = val
	}
	if val := os.Getenv("MD_AI_OLLAMA_HOST"); val != "" {
		c.OllamaHost = val
	}
	if val := os.Getenv("MD_AI_MODEL"); val != "" {
		c.Model = val
	}
	if val := os.Getenv("MD_AI_TEMPERATURE"); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			c.Temperature = f
		}
	}
	if val := os.Getenv("MD_AI_MAX_TOKENS"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			c.MaxTokens = i
		}
	}
	if val := os.Getenv("MD_AI_MAX_CONTEXT_TOKENS"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			c.MaxContextTokens = i
		}
	}
	if val := os.Getenv("MD_AI_GRAPH_DEPTH"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			c.GraphDepth = i
		}
	}
	if val := os.Getenv("MD_AI_TIMEOUT"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			c.TimeoutSeconds = i
		}
	}
	if val := os.Getenv("MD_AI_JSON"); val != "" {
		c.JSONOutput = val == "true" || val == "1"
	}
}

// Validate checks whether configuration values are within acceptable bounds.
func (c *Config) Validate() error {
	if c.DBPath == "" {
		return fmt.Errorf("database path must not be empty")
	}
	if c.OllamaHost == "" {
		return fmt.Errorf("ollama host must not be empty")
	}
	if c.Model == "" {
		return fmt.Errorf("model name must not be empty")
	}
	if c.GraphDepth < 1 {
		return fmt.Errorf("graph depth must be at least 1")
	}
	if c.MaxContextTokens < 100 {
		return fmt.Errorf("max context tokens must be at least 100")
	}
	if c.TimeoutSeconds < 1 {
		return fmt.Errorf("timeout must be at least 1 second")
	}
	return nil
}

// Timeout returns the LLM request timeout as a duration.
func (c *Config) Timeout() time.Duration {
	return time.Duration(c.TimeoutSeconds) * time.Second
}
