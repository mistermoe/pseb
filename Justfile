# Show available commands
_help:
    @just -l

lint:
  @echo "Running linter..."
  @golangci-lint run

test:
  @echo "Running tests..."
  @go clean -testcache && go test -cover ./...

# Extract and verify a PSEB certificate PDF
verify path:
  #!/bin/bash
  set -euo pipefail

  go run ./cmd/pseb {{path}}


docs:
  #!/bin/bash
  set -euo pipefail

  cd docs
  pnpm install
  pnpm start