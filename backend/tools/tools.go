// Package tools documents the pinned dev-tool binaries for this module.
// Tools are tracked via `go get -tool` (Go 1.24+ `tool` directives in
// go.mod), not blank imports — see go.mod's `tool` line and
// `internal/shared/api/generate.go` for how oapi-codegen is invoked.
//
// sqlc and golangci-lint are added the same way when Phase 4/CI needs them.
package tools
