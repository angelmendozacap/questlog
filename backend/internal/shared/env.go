package shared

import (
	"log"
	"os"
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
