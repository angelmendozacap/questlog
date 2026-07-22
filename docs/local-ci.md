# Running CI locally

Equivalent commands for each job in `.github/workflows/ci.yml`, so you can catch
failures before pushing.

## 1. Contract drift (OpenAPI → TS + Go types)

```bash
pnpm --filter @questlog/types generate
git diff --exit-code -- packages/types/src/generated   # fails if dirty = drift

cd backend/internal/shared/api && go generate ./... && cd ../../../..
git diff --exit-code -- backend/internal/shared/api/generated
```

## 2. TS lint, typecheck

```bash
pnpm turbo lint typecheck
# or individually: pnpm turbo lint / pnpm turbo typecheck
```

## 3. Go lint, vet, test

```bash
cd backend
go build ./... && go vet ./... && go test ./... -race
go tool golangci-lint run ./...   # pinned via backend/tools/go.mod, no separate install needed
```

## 4. Docker build matrix

Requires Docker (not installed on this machine yet — see note below).

```bash
docker build -f deploy/Dockerfile.go   --build-arg CMD_PATH=cmd/public-api .
docker build -f deploy/Dockerfile.go   --build-arg CMD_PATH=cmd/admin-api .
docker build -f deploy/Dockerfile.node --build-arg APP=web .
docker build -f deploy/Dockerfile.node --build-arg APP=admin .
```

## Running the *actual* GitHub Actions workflow locally

[`act`](https://github.com/nektos/act) replays `.github/workflows/ci.yml` itself
inside Docker, closest thing to "what CI will actually do":

```bash
brew install act
act push          # runs the same jobs push triggers
```

Also requires Docker — same gap as job 4.

## Docker gap

Docker isn't installed on this machine. Install **Docker Desktop** (macOS) to unlock
job 4 and `act`. Everything else (jobs 1–3, which is most of what actually breaks)
runs without it.
