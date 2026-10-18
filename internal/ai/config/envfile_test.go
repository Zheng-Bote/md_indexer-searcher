// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeEnv(t *testing.T, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadEnvFile_ProvidersAndModels(t *testing.T) {
	path := writeEnv(t, `# cloud providers
CLAUDE_API_KEY="sk-ant-test"
CLAUDE_LLM_02=claude-sonnet-5-5
CLAUDE_LLM_01=claude-opus-5-5
export OPENROUTER_API_KEY='or-key'
OPENROUTER_ENDPOINT=https://openrouter.example/api/v1/
OPENROUTER_LLM_01=deepseek/deepseek-r1:free
GEMINI_API_KEY=NONE
GEMINI_LLM_01=gemini-3.8-flash
NVIDIA_API_KEY=
GROQ_API_KEY=groq-key
GROQ_LLM_01=llama
`, 0o600)

	ef, err := LoadEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if ef.Insecure {
		t.Error("0600 file reported as insecure")
	}
	providers := ef.Providers()
	if len(providers) != 2 {
		t.Fatalf("expected claude and openrouter only, got %+v", providers)
	}
	byID := map[string]ProviderConfig{}
	for _, p := range providers {
		byID[p.ID] = p
	}
	claude, openrouter := byID[ProviderClaude], byID[ProviderOpenRouter]
	if claude.ID != ProviderClaude || claude.APIKey != "sk-ant-test" || claude.Protocol != ProtocolClaude {
		t.Errorf("claude config %+v", claude)
	}
	if strings.Join(claude.Models, ",") != "claude-opus-5-5,claude-sonnet-5-5" {
		t.Errorf("claude models not in key order: %v", claude.Models)
	}
	if openrouter.APIKey != "or-key" || openrouter.Endpoint != "https://openrouter.example/api/v1" {
		t.Errorf("openrouter config %+v", openrouter)
	}
}

func TestLoadEnvFile_ProcessEnvWinsAndDefaults(t *testing.T) {
	path := writeEnv(t, "GEMINI_API_KEY=file-key\nGEMINI_LLM_01=gemini-3.8-flash\n", 0o644)
	t.Setenv("GEMINI_API_KEY", "env-key")

	ef, err := LoadEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !ef.Insecure {
		t.Error("0644 file must be reported as insecure")
	}
	p := ef.Providers()
	if len(p) != 1 || p[0].APIKey != "env-key" {
		t.Fatalf("process environment must win, got %+v", p)
	}
	if p[0].Endpoint != "https://generativelanguage.googleapis.com/v1beta/openai" {
		t.Errorf("default gemini endpoint expected, got %s", p[0].Endpoint)
	}
}

func TestLoadEnvFile_MissingFiles(t *testing.T) {
	if _, err := LoadEnvFile(filepath.Join(t.TempDir(), "missing.env")); err == nil {
		t.Error("explicit missing file must be an error")
	}
	t.Setenv("HOME", t.TempDir())
	ef, err := LoadEnvFile("")
	if err != nil || len(ef.Vars) != 0 {
		t.Errorf("missing default file must yield an empty env file, got %+v, %v", ef, err)
	}
}

func TestConfig_ClaudeEffort(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DBPath = "x.db"
	cfg.ClaudeEffortByModel = map[string]string{"claude-opus-5-5": "high"}
	if cfg.EffortFor("claude-opus-5-5") != "high" || cfg.EffortFor("claude-haiku-5-5") != "medium" {
		t.Errorf("unexpected efforts %s / %s", cfg.EffortFor("claude-opus-5-5"), cfg.EffortFor("claude-haiku-5-5"))
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg.ClaudeEffort = "extreme"
	if err := cfg.Validate(); err == nil {
		t.Error("invalid effort must fail validation")
	}
}
