package config

import (
	"strings"
	"testing"
)

func mustLoad(t *testing.T, kvs ...string) *Config {
	t.Helper()
	// Always include the required minimum so tests focus on what they override.
	defaults := map[string]string{
		"WAHA_URL":              "http://waha:3000",
		"WAHA_CHAT_ID":          "1203@g.us",
		"WAHA_BOT_PHONE":        "+41 79 111 22 33",
		"SEERR_URL":             "http://seerr:5055",
		"SEERR_API_KEY":         "k",
		"SONARR_URL":            "http://sonarr:8989",
		"SONARR_API_KEY":        "k",
		"RADARR_URL":            "http://radarr:7878",
		"RADARR_API_KEY":        "k",
		"JELLYFIN_URL":          "http://jellyfin:8096",
		"JELLYFIN_API_KEY":      "k",
		"JELLYFIN_USER_ID":      "abc",
		"JELLYFIN_EXTERNAL_URL": "https://jf.test",
		"SEERR_EXTERNAL_URL":    "https://seerr.test",
	}
	for _, kv := range kvs {
		if eq := strings.IndexByte(kv, '='); eq > 0 {
			defaults[kv[:eq]] = kv[eq+1:]
		}
	}
	env := make([]string, 0, len(defaults))
	for k, v := range defaults {
		env = append(env, k+"="+v)
	}
	c, err := Load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return c
}

func TestLoadDefaults(t *testing.T) {
	c := mustLoad(t)
	if c.Listen != ":8080" {
		t.Errorf("Listen default = %q", c.Listen)
	}
	if c.WAHASession != "default" {
		t.Errorf("WAHASession default = %q", c.WAHASession)
	}
	// Phone is normalized to digits only.
	if c.WAHABotPhone != "41791112233" {
		t.Errorf("WAHABotPhone = %q, want digits-only", c.WAHABotPhone)
	}
	if c.MentionToken() != "@41791112233" {
		t.Errorf("MentionToken = %q", c.MentionToken())
	}
}

func TestLoadValidationMissing(t *testing.T) {
	if _, err := Load(nil); err == nil {
		t.Fatal("expected validation error when env is empty")
	}
}

func TestPhoneMap(t *testing.T) {
	c := mustLoad(t,
		"PHONE_MAP_PATRIK=+41 79 111 22 33",
		"PHONE_MAP_ADRIAN=41 79 999 88 77",
	)
	if c.PhoneMap["patrik"] != "41791112233" {
		t.Errorf("patrik = %q", c.PhoneMap["patrik"])
	}
	if c.PhoneMap["adrian"] != "41799998877" {
		t.Errorf("adrian = %q", c.PhoneMap["adrian"])
	}
}

func TestLogLevelValidation(t *testing.T) {
	if _, err := Load(toEnv(map[string]string{
		"WAHA_URL": "http://x", "WAHA_CHAT_ID": "x", "WAHA_BOT_PHONE": "1",
		"SEERR_URL": "x", "SEERR_API_KEY": "x", "SONARR_URL": "x", "SONARR_API_KEY": "x",
		"RADARR_URL": "x", "RADARR_API_KEY": "x", "JELLYFIN_URL": "x", "JELLYFIN_API_KEY": "x",
		"JELLYFIN_USER_ID": "x", "JELLYFIN_EXTERNAL_URL": "x", "SEERR_EXTERNAL_URL": "x",
		"LOG_LEVEL": "loud",
	})); err == nil {
		t.Fatal("expected error on bad LOG_LEVEL")
	}
}

func toEnv(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}
