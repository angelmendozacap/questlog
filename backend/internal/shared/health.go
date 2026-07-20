// Package shared holds the shared kernel: cross-context errors, pagination
// helpers, and domain event plumbing (Phase 5+). For now it just exposes the
// health payload both API binaries return.
package shared

// Health mirrors components.schemas.Health in api/openapi.yaml.
type Health struct {
	Status string `json:"status"`
}

// OK returns the canonical healthy response.
func OK() Health {
	return Health{Status: "ok"}
}
