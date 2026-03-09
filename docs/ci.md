# CI Checks

cloudTalk CI is defined in `.github/workflows/ci.yml` and runs on both `push`
and `pull_request` events.

Each job runs on `ubuntu-latest`, checks out the repository, and uses
`actions/setup-go@v5` with `go-version-file: go.mod` and module cache enabled.

## What CI validates

### 1) Lint

Runs in this order:

1. `gofmt` formatting check
2. `go vet ./...`
3. `golangci-lint` with rules from `.golangci.yml`

Lint job settings:

- Timeout: 10 minutes
- `golangci/golangci-lint-action@v6`
- `golangci-lint` version `v1.64.8`
- Lint args: `--timeout=5m`

Enabled lint rules include:

- `errcheck`
- `gosimple`
- `govet`
- `ineffassign`
- `staticcheck`
- `unused`
- `unconvert`
- `misspell`
- `nolintlint`
- `bodyclose`
- `noctx`
- `revive`
- `gocritic`
- `whitespace`

### 2) Unit tests

- `go test ./... -count=1`
- Timeout: 15 minutes

### 3) Race tests

- `go test -race ./... -count=1`
- Timeout: 20 minutes

### 4) Integration tests

- `go test -tags=integration ./... -count=1`
- Timeout: 30 minutes

## Notes

- PRs may show duplicate checks because CI runs for both `push` and `pull_request`.
- Lint, unit, race, and integration checks are expected to pass before merge.
