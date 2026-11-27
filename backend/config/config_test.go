// SPDX-License-Identifier: AGPL-3.0-or-later
// SPDX-FileCopyrightText: 2026 ItsWempo

package config

import (
	"strings"
	"testing"
)

func validProductionConfig() *Config {
	return &Config{
		AppEnv: "production", PublicBaseURL: "https://vertexia.wempos-forum.com",
		RunWithDatabase: true, DBHost: "db.example.net", DBUser: "vertexia", DBPass: "secret", DBName: "vertexia", DBTLS: true,
		SessionSecure: true, RedisRequired: true, RedisURL: "rediss://redis.example.net:6379",
		AltchaHMACKey: strings.Repeat("a", 32), RegistrationIPSalt: strings.Repeat("b", 32),
		R2Bucket: "vertexia-assets", R2AccountID: "account", R2AccessKeyID: "access", R2SecretAccessKey: "secret",
	}
}

func TestValidateAcceptsDurableProductionConfig(t *testing.T) {
	if err := validProductionConfig().Validate(); err != nil {
		t.Fatalf("valid production config rejected: %v", err)
	}
}

func TestValidateRejectsEphemeralProductionState(t *testing.T) {
	cfg := validProductionConfig()
	cfg.RedisRequired = false
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "REDIS_REQUIRED") {
		t.Fatalf("expected Redis durability validation, got %v", err)
	}
}

func TestValidateRejectsPlatformHostnameAsUDPHost(t *testing.T) {
	cfg := validProductionConfig()
	cfg.GSMEnabled = true
	cfg.GSMAgentURL = "https://agent.example.net"
	cfg.GSMAgentToken = strings.Repeat("t", 32)
	cfg.GSMPublicHost = "vertexia.wempos-forum.com"
	cfg.NetcodePrivateKey = make([]byte, 32)
	cfg.NetcodeProtocolID = 1
	cfg.GameserverAPIKey = strings.Repeat("k", 32)
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "must not be the platform web hostname") {
		t.Fatalf("expected separate UDP hostname validation, got %v", err)
	}
}
