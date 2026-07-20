// Command migrate applies golang-migrate migrations found in
// backend/migrations against DATABASE_URL. Wired for real in Phase 2's
// Docker Compose step; placeholder for now so `go build ./...` covers it.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("questlog migrate: not wired yet (Phase 2 — Docker Compose step)")
	os.Exit(0)
}
