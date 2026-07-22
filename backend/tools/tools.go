// Package tools documents the pinned dev-tool binaries for this module.
// Tools are tracked via `go get -tool` (Go 1.24+ `tool` directives in
// go.mod), not blank imports — see go.mod's `tool` lines.
//
// Pinned so far:
//   - oapi-codegen — internal/shared/api/generate.go (`go generate ./...`)
//   - golangci-lint — CI runs `go tool golangci-lint run ./...` from backend/
//
// sqlc joins the same way when Phase 4 needs it.
package tools
