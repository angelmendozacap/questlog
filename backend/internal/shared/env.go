package shared

import (
	"log"
	"os"
	"strings"
)

// MustEnv returns the environment variable's value or exits the process —
// used by cmd/* binaries for required startup configuration.
func MustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required env var %s", key)
	}
	return v
}

// MustEnvList returns a required comma-separated environment variable split
// into its trimmed, non-empty elements — used for allow-lists like
// KEYCLOAK_ALLOWED_AZP where a binary needs to name more than one value.
func MustEnvList(key string) []string {
	raw := MustEnv(key)
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		log.Fatalf("env var %s must contain at least one value", key)
	}
	return out
}
