#!/usr/bin/env bash
# =============================================================================
# SurplusToken — Deploy to Clab
#
# Usage:
#   ./scripts/deploy.sh <clab-host>
#
# Example:
#   ./scripts/deploy.sh root@192.168.1.100
#   ./scripts/deploy.sh user@my-server.example.com:/opt/surplustoken
#
# Prerequisites on clab:
#   - Docker 20.10+ with docker compose plugin
#   - SSH access with key-based auth
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DEPLOY_DIR="${PROJECT_DIR}/deploy"

# ---- Parse arguments ----
CLAB_TARGET="${1:-}"
if [ -z "$CLAB_TARGET" ]; then
    echo "Usage: $0 <clab-host>"
    echo "  clab-host: SSH target, e.g. root@192.168.1.100 or user@host:/opt/surplustoken"
    echo ""
    echo "Environment variables:"
    echo "  CLAB_HOST  — default SSH host if not provided as argument"
    exit 1
fi

# Extract host and optional remote path
if [[ "$CLAB_TARGET" == *:* ]]; then
    CLAB_HOST="${CLAB_TARGET%%:*}"
    REMOTE_PATH="${CLAB_TARGET#*:}"
else
    CLAB_HOST="$CLAB_TARGET"
    REMOTE_PATH="/opt/surplustoken"
fi

echo "========================================"
echo " SurplusToken Deployment"
echo "========================================"
echo "  Target:  ${CLAB_HOST}"
echo "  Path:    ${REMOTE_PATH}"
echo "========================================"

# ---- Step 1: Prepare deployment archive ----
echo ""
echo "[1/5] Preparing deployment files..."

# Create temp archive of the project (without git history and large files)
TMP_ARCHIVE="/tmp/surplustoken-deploy.tar.gz"
cd "$PROJECT_DIR"
tar czf "$TMP_ARCHIVE" \
    --exclude='.git' \
    --exclude='node_modules' \
    --exclude='.pnpm-store' \
    --exclude='*.log' \
    --exclude='.DS_Store' \
    config/ \
    deploy/ \
    new-api/ \
    cpa/config.example.yaml \
    scripts/ \
    2>/dev/null || true

ARCHIVE_SIZE=$(du -h "$TMP_ARCHIVE" | cut -f1)
echo "  Archive size: ${ARCHIVE_SIZE}"

# ---- Step 2: Create remote directory ----
echo ""
echo "[2/5] Creating remote directory..."
ssh "$CLAB_HOST" "mkdir -p ${REMOTE_PATH}"

# ---- Step 3: Upload archive ----
echo ""
echo "[3/5] Uploading to clab..."
scp "$TMP_ARCHIVE" "${CLAB_HOST}:${REMOTE_PATH}/deploy.tar.gz"

# ---- Step 4: Extract on remote ----
echo ""
echo "[4/5] Extracting on clab..."
ssh "$CLAB_HOST" << EOF
    set -e
    cd ${REMOTE_PATH}
    tar xzf deploy.tar.gz
    rm deploy.tar.gz

    # Copy .env if it doesn't exist
    if [ ! -f deploy/.env ]; then
        cp deploy/.env.example deploy/.env
        echo ""
        echo "⚠️  deploy/.env created from template."
        echo "   Edit ${REMOTE_PATH}/deploy/.env on the server before starting!"
        echo ""
    fi

    # Make scripts executable
    chmod +x scripts/*.sh 2>/dev/null || true
EOF

echo "  Extracted to ${REMOTE_PATH}"

# ---- Step 5: Build and start ----
echo ""
echo "[5/5] Starting services..."
echo ""

cat << 'INSTRUCTIONS'
========================================
 Next steps — run these on the clab server:
========================================

  # SSH into the server
  ssh HOST

  # Edit environment variables (REQUIRED!)
  cd /opt/surplustoken/deploy
  nano .env

  # Build and start all services
  docker compose up -d --build

  # View logs
  docker compose logs -f

  # Check service health
  docker compose ps

  # Access the web UI
  # http://<server-ip>:3000

  # Initial setup:
  # 1. Open http://<server-ip>:3000
  # 2. Create root admin account (first run only)
  # 3. Go to /oauth-accounts to connect OAuth providers
  # 4. Configure channels for Chinese models (DeepSeek/GLM/Kimi)

========================================
INSTRUCTIONS

# Clean up local archive
rm -f "$TMP_ARCHIVE"

echo "Deployment archive uploaded successfully!"
