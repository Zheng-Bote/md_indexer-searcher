// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Provider IDs used in the model list and in model references.
const (
	ProviderOllama      = "ollama"
	ProviderOllamaCloud = "ollama-cloud"
	ProviderOpenRouter  = "openrouter"
	ProviderNVIDIA      = "nvidia"
	ProviderClaude      = "claude"
	ProviderGemini      = "gemini"
)

// Protocols spoken by cloud providers.
const (
	ProtocolOllama = "ollama"
	ProtocolOpenAI = "openai"
	ProtocolClaude = "claude"
)

// ProviderConfig describes one cloud provider enabled in the .env file.
type ProviderConfig struct {
	ID          string
	DisplayName string
	Protocol    string
	APIKey      string
	Endpoint    string
	Models      []string
}

type providerSpec struct {
	prefix, id, name, protocol, endpoint string
}

// cloudProviders lists the supported .env prefixes; others (e.g. GROQ) are ignored.
var cloudProviders = []providerSpec{
	{"OLLAMA", ProviderOllamaCloud, "Ollama Cloud", ProtocolOllama, "https://ollama.com"},
	{"OPENROUTER", ProviderOpenRouter, "OpenRouter", ProtocolOpenAI, "https://openrouter.ai/api/v1"},
	{"NVIDIA", ProviderNVIDIA, "NVIDIA NIM", ProtocolOpenAI, "https://integrate.api.nvidia.com/v1"},
	{"CLAUDE", ProviderClaude, "Claude", ProtocolClaude, ""},
	{"GEMINI", ProviderGemini, "Gemini", ProtocolOpenAI, "https://generativelanguage.googleapis.com/v1beta/openai"},
}

// EnvFile holds the variables of md_indexer's .env file.
type EnvFile struct {
	Path string
	Vars map[string]string
	// Insecure is true when the file is readable by group or others.
	Insecure bool
}

// DefaultEnvFilePath returns ~/.config/md-indexer/.env.
func DefaultEnvFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "md-indexer", ".env")
}

// LoadEnvFile reads path, or the default path when path is empty. A missing default file yields an
// empty EnvFile; a missing explicit file is an error.
func LoadEnvFile(path string) (*EnvFile, error) {
	explicit := path != ""
	if !explicit {
		path = DefaultEnvFilePath()
	}
	ef := &EnvFile{Path: path, Vars: map[string]string{}}
	if path == "" {
		return ef, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if !explicit && errors.Is(err, os.ErrNotExist) {
			return ef, nil
		}
		return nil, fmt.Errorf("open env file: %w", err)
	}
	defer f.Close()
	if info, err := f.Stat(); err == nil && info.Mode().Perm()&0o077 != 0 {
		ef.Insecure = true
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		ef.Vars[strings.TrimSpace(key)] = unquote(strings.TrimSpace(value))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read env file: %w", err)
	}
	return ef, nil
}

func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}

// Lookup returns the value of key; the process environment wins over the file.
func (e *EnvFile) Lookup(key string) (string, bool) {
	if v, ok := os.LookupEnv(key); ok {
		return v, true
	}
	v, ok := e.Vars[key]
	return v, ok
}

// keys returns all variable names from the file and the process environment.
func (e *EnvFile) keys() []string {
	seen := make(map[string]bool, len(e.Vars))
	for k := range e.Vars {
		seen[k] = true
	}
	for _, kv := range os.Environ() {
		if k, _, ok := strings.Cut(kv, "="); ok {
			seen[k] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Providers returns the cloud providers whose API key is set and not "none", with their models
// from <PREFIX>_LLM_xx in key order.
func (e *EnvFile) Providers() []ProviderConfig {
	keys := e.keys()
	var out []ProviderConfig
	for _, spec := range cloudProviders {
		key, _ := e.Lookup(spec.prefix + "_API_KEY")
		key = strings.TrimSpace(key)
		if key == "" || strings.EqualFold(key, "none") {
			continue
		}
		endpoint, _ := e.Lookup(spec.prefix + "_ENDPOINT")
		if endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/"); endpoint == "" {
			endpoint = spec.endpoint
		}
		pc := ProviderConfig{ID: spec.id, DisplayName: spec.name, Protocol: spec.protocol, APIKey: key, Endpoint: endpoint}
		for _, k := range keys {
			if strings.HasPrefix(k, spec.prefix+"_LLM_") {
				if m, _ := e.Lookup(k); strings.TrimSpace(m) != "" {
					pc.Models = append(pc.Models, strings.TrimSpace(m))
				}
			}
		}
		out = append(out, pc)
	}
	return out
}
