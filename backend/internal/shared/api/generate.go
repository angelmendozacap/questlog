// Package api holds Go types generated from api/openapi.yaml — the Go side
// of the contract (packages/types is the TS side). Both API binaries import
// this package for request/response shapes; CI fails the build if the
// generated file drifts from the committed spec (see .github/workflows/ci.yml).
package api

//go:generate go tool oapi-codegen -config oapi-codegen.yaml ../../../../api/openapi.yaml
