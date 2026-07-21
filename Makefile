.PHONY: build run dev clean frontend

# Build frontend
frontend:
	cd web && npm install && npm run build

# Build backend with embedded frontend
build: frontend
	CGO_ENABLED=1 go build -o k2board ./cmd/server

# Build backend only (for development)
build-server:
	CGO_ENABLED=1 go build -o k2board ./cmd/server

# Run development server (with hot-reload via Air if available)
dev:
	go run ./cmd/server

# Run with frontend dev server
dev-frontend:
	cd web && npm run dev

# Clean build artifacts
clean:
	rm -f k2board k2board.db
	rm -rf web/dist

# Run tests
test:
	go test ./...
