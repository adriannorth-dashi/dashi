// Copyright (C) 2025 Dashi Project
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"os"
	"testing"
)

func TestGetEnv_ReturnsSetValue(t *testing.T) {
	const key, want = "DASHI_TEST_VAR", "hello"
	os.Setenv(key, want)
	defer os.Unsetenv(key)

	if got := getEnv(key, "fallback"); got != want {
		t.Errorf("getEnv(%q) = %q, want %q", key, got, want)
	}
}

func TestGetEnv_ReturnsFallbackWhenUnset(t *testing.T) {
	const key = "DASHI_TEST_UNSET_VAR"
	os.Unsetenv(key)

	if got := getEnv(key, "fallback"); got != "fallback" {
		t.Errorf("getEnv(%q) = %q, want %q", key, got, "fallback")
	}
}

func TestGetEnv_ReturnsFallbackForEmptyValue(t *testing.T) {
	const key = "DASHI_TEST_EMPTY_VAR"
	os.Setenv(key, "")
	defer os.Unsetenv(key)

	// Empty string counts as unset → fallback is returned.
	if got := getEnv(key, "fallback"); got != "fallback" {
		t.Errorf("getEnv(%q) with empty value = %q, want fallback", key, got)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	// Hide .env so godotenv.Load() has no effect during this test.
	if err := os.Rename(".env", ".env.test_bak"); err == nil {
		defer os.Rename(".env.test_bak", ".env")
	}

	// Unset all Dashi env vars so we get pure code-level defaults.
	vars := []string{"PORT", "SUI_NETWORK", "SUI_RPC_URL", "GASPOOL_URL", "GASPOOL_AUTH_TOKEN", "SPONSOR_ADDRESS", "API_KEY", "DATABASE_URL", "REDIS_URL"}
	saved := make(map[string]string, len(vars))
	for _, v := range vars {
		saved[v] = os.Getenv(v)
		os.Unsetenv(v)
	}
	defer func() {
		for k, v := range saved {
			if v != "" {
				os.Setenv(k, v)
			}
		}
	}()

	cfg := loadConfig()

	if cfg.Port != "8080" {
		t.Errorf("default Port = %q, want 8080", cfg.Port)
	}
	if cfg.Network != "mainnet" {
		t.Errorf("default Network = %q, want mainnet", cfg.Network)
	}
	if cfg.RPCURL != "https://fullnode.mainnet.sui.io:443" {
		t.Errorf("default RPCURL = %q", cfg.RPCURL)
	}
	if cfg.GasPoolURL != "http://127.0.0.1:9527" {
		t.Errorf("default GasPoolURL = %q, want http://127.0.0.1:9527", cfg.GasPoolURL)
	}
}

func TestLoadConfig_ReadsEnvVars(t *testing.T) {
	os.Setenv("PORT", "9090")
	os.Setenv("SUI_NETWORK", "mainnet")
	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("SUI_NETWORK")
	}()

	cfg := loadConfig()

	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want 9090", cfg.Port)
	}
	if cfg.Network != "mainnet" {
		t.Errorf("Network = %q, want mainnet", cfg.Network)
	}
}
