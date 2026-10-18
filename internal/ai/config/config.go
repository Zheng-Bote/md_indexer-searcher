// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gopkg.in/yaml.v2"
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

	// Retriever selects the retrieval implementation: RetrieverHybrid or RetrieverClassic.
	Retriever          string  `json:"retriever" yaml:"retriever"`
	TopKFTS            int     `json:"top_k_fts" yaml:"top_k_fts"`
	FusionFTSWeight    float64 `json:"fusion_fts_weight" yaml:"fusion_fts_weight"`
	FusionGraphWeight  float64 `json:"fusion_graph_weight" yaml:"fusion_graph_weight"`
	FusionEntityWeight float64 `json:"fusion_entity_weight" yaml:"fusion_entity_weight"`

	// Vector retrieval uses section embeddings written by the indexer (-embed-model) with the same model.
	EnableVectors      bool    `json:"enable_vectors" yaml:"enable_vectors"`
	EmbeddingModel     string  `json:"embedding_model" yaml:"embedding_model"`
	TopKVector         int     `json:"top_k_vector" yaml:"top_k_vector"`
	MinVectorScore     float64 `json:"min_vector_score" yaml:"min_vector_score"`
	FusionVectorWeight float64 `json:"fusion_vector_weight" yaml:"fusion_vector_weight"`
}

// Retriever implementations.
const (
	RetrieverHybrid  = "hybrid"
	RetrieverClassic = "classic"
)

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		// Empty: the tools resolve the vault's index database (see internal/vaultdb).
		DBPath:           "",
		OllamaHost:       "http://localhost:11434",
		Model:            "qwen3:14b",
		Temperature:      0.2,
		MaxTokens:        2048,
		MaxContextTokens: 4000,
		GraphDepth:       3,
		TimeoutSeconds:   600,
		JSONOutput:       false,

		Retriever:          RetrieverHybrid,
		TopKFTS:            25,
		FusionFTSWeight:    0.35,
		FusionGraphWeight:  0.20,
		FusionEntityWeight: 0.10,

		EnableVectors:      true,
		EmbeddingModel:     "bge-m3",
		TopKVector:         25,
		MinVectorScore:     0.55,
		FusionVectorWeight: 0.35,
	}
}

// LoadFromFile reads YAML configuration from path and merges non-empty values into c.
func (c *Config) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, c)
}

// LoadDefaultFiles loads ~/.config/md-indexer/ai.yaml and ./md-ai.yaml if they exist,
// and then applies MD_AI_CONFIG if set.
func (c *Config) LoadDefaultFiles() error {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		userCfg := filepath.Join(home, ".config", "md-indexer", "ai.yaml")
		if _, err := os.Stat(userCfg); err == nil {
			if err := c.LoadFromFile(userCfg); err != nil {
				return fmt.Errorf("failed to load %s: %w", userCfg, err)
			}
		}
	}
	if _, err := os.Stat("md-ai.yaml"); err == nil {
		if err := c.LoadFromFile("md-ai.yaml"); err != nil {
			return fmt.Errorf("failed to load md-ai.yaml: %w", err)
		}
	}
	if envPath := os.Getenv("MD_AI_CONFIG"); envPath != "" {
		if err := c.LoadFromFile(envPath); err != nil {
			return fmt.Errorf("failed to load %s: %w", envPath, err)
		}
	}
	return nil
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
	if val := os.Getenv("MD_AI_RETRIEVER"); val != "" {
		c.Retriever = val
	}
	if val := os.Getenv("MD_AI_TOP_K_FTS"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			c.TopKFTS = i
		}
	}
	if val := os.Getenv("MD_AI_ENABLE_VECTORS"); val != "" {
		c.EnableVectors = val == "true" || val == "1"
	}
	if val := os.Getenv("MD_AI_EMBEDDING_MODEL"); val != "" {
		c.EmbeddingModel = val
	}
	if val := os.Getenv("MD_AI_TOP_K_VECTOR"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			c.TopKVector = i
		}
	}
	if val := os.Getenv("MD_AI_MIN_VECTOR_SCORE"); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			c.MinVectorScore = f
		}
	}
	if val := os.Getenv("MD_AI_FUSION_FTS_WEIGHT"); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			c.FusionFTSWeight = f
		}
	}
	if val := os.Getenv("MD_AI_FUSION_VECTOR_WEIGHT"); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			c.FusionVectorWeight = f
		}
	}
	if val := os.Getenv("MD_AI_FUSION_GRAPH_WEIGHT"); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			c.FusionGraphWeight = f
		}
	}
	if val := os.Getenv("MD_AI_FUSION_ENTITY_WEIGHT"); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			c.FusionEntityWeight = f
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
	if c.EnableVectors && c.EmbeddingModel == "" {
		return fmt.Errorf("embedding model must not be empty when vectors are enabled")
	}
	if c.Retriever != RetrieverHybrid && c.Retriever != RetrieverClassic {
		return fmt.Errorf("retriever must be %q or %q, got %q", RetrieverHybrid, RetrieverClassic, c.Retriever)
	}
	return nil
}

// Timeout returns the LLM request timeout as a duration.
func (c *Config) Timeout() time.Duration {
	return time.Duration(c.TimeoutSeconds) * time.Second
}
