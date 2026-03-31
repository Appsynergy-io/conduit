.PHONY: all build-web build-server build-agent dev-server lint test clean generate

all: build-server build-agent

# Frontend: Next.js static export → web/out/
build-web:
	cd web && pnpm install --frozen-lockfile && pnpm build

# Server binary (embeds frontend from web/out/)
build-server: build-web
	go build -o conduit-server ./cmd/conduit-server

# Agent/CLI binary (no CGO, cross-platform)
build-agent:
	CGO_ENABLED=0 go build -o conduit ./cmd/conduit

# Dev server (self-signed TLS, port 8443)
dev-server:
	go run ./cmd/conduit-server --dev

# Generate API types + server interface from openapi.yaml
generate:
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest \
		--config oapi-codegen.yaml openapi.yaml

# Go linter
lint:
	golangci-lint run ./...
	cd web && pnpm lint

# Go + frontend tests
test:
	go test ./...
	cd web && pnpm test

clean:
	rm -f conduit-server conduit
	rm -rf web/out web/.next
