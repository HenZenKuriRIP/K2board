#!/bin/bash
# Build release artifacts:
#   k2board-linux-amd64 / k2board-linux-arm64  (panel binary, admin UI embedded)
#   k2board-user-dist.tar.gz                   (standalone user portal static)
#
# Usage: ./build_release.sh v1.4.27
# Optional: VITE_API_BASE=https://www.example.com  (baked into user dist;
#           leave empty and use runtime config.js on shadow hosts)
set -euo pipefail

VERSION=${1:-v1.0.0}
BUILD_DIR="release_builds"
rm -rf "$BUILD_DIR" && mkdir -p "$BUILD_DIR"

echo "=== Build admin UI (embed) ==="
(cd web && npm install && npm run build)

echo "=== Build user portal (standalone dist) ==="
(
  cd web_user
  npm install
  # Empty VITE_API_BASE → shadow hosts set window.__K2_API_BASE__ via config.js
  # Or pass VITE_API_BASE=https://www.panel.com to bake it in.
  if [[ -n "${VITE_API_BASE:-}" ]]; then
    echo "  VITE_API_BASE=${VITE_API_BASE}"
    VITE_API_BASE="$VITE_API_BASE" npm run build
  else
    npm run build
  fi
)

# Ensure config.js exists in dist (runtime API base for multi-origin)
if [[ ! -f web_user/dist/config.js ]]; then
  if [[ -f web_user/public/config.js ]]; then
    cp web_user/public/config.js web_user/dist/config.js
  else
    cat > web_user/dist/config.js <<'EOF'
// Set on each shadow host, e.g. window.__K2_API_BASE__ = 'https://www.example.com';
EOF
  fi
fi
cp -f web_user/public/config.example.js web_user/dist/config.example.js 2>/dev/null || true

echo "=== Pack k2board-user-dist.tar.gz ==="
tar -czf "$BUILD_DIR/k2board-user-dist.tar.gz" -C web_user/dist .
ls -lh "$BUILD_DIR/k2board-user-dist.tar.gz"

# Go 架构 → Zig 架构映射
zig_arch() {
  case "$1" in
    amd64) echo "x86_64" ;;
    arm64) echo "aarch64" ;;
    *)     echo "$1" ;;
  esac
}

targets=(
  "linux/amd64"
  "linux/arm64"
)

for t in "${targets[@]}"; do
  GOOS="${t%/*}"
  GOARCH="${t#*/}"
  output="k2board-${GOOS}-${GOARCH}"
  ZIG_ARCH=$(zig_arch "$GOARCH")

  echo "=== Build $output ==="

  if command -v zig >/dev/null 2>&1; then
    CGO_ENABLED=1 \
    GOOS="$GOOS" GOARCH="$GOARCH" \
    CC="zig cc -target ${ZIG_ARCH}-${GOOS}-musl" \
    CXX="zig c++ -target ${ZIG_ARCH}-${GOOS}-musl" \
    go build \
      -ldflags="-linkmode=external -extldflags=-static" \
      -o "$BUILD_DIR/$output" \
      ./cmd/server
  else
    echo "  (zig not found — building without static CGO link)"
    CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
      go build -o "$BUILD_DIR/$output" ./cmd/server
  fi

  echo "  -> $BUILD_DIR/$output done"
done

echo "=== Artifacts ($VERSION) ==="
ls -lh "$BUILD_DIR"
echo "Upload with: gh release create $VERSION $BUILD_DIR/* --title \"K2Board $VERSION\" --notes-file -"
