#!/usr/bin/env bash
set -euo pipefail

FLOWPULSE_VERSION="${FLOWPULSE_VERSION:-latest}"
REPO_URL="https://github.com/KAPS3000/flowpulse.ai"
INSTALL_DIR="${FLOWPULSE_HOME:-$HOME/flowpulse}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

banner() {
  echo ""
  echo -e "${CYAN}${BOLD}"
  echo "  ╔═══════════════════════════════════════════════════╗"
  echo "  ║                                                   ║"
  echo "  ║   FlowPulse — ML Training Cluster Monitor         ║"
  echo "  ║   eBPF-Powered Network & GPU Observability        ║"
  echo "  ║                                                   ║"
  echo "  ╚═══════════════════════════════════════════════════╝"
  echo -e "${NC}"
}

log()   { echo -e "${GREEN}[FlowPulse]${NC} $*"; }
warn()  { echo -e "${YELLOW}[FlowPulse]${NC} $*"; }
error() { echo -e "${RED}[FlowPulse]${NC} $*" >&2; }

check_dependency() {
  if ! command -v "$1" &>/dev/null; then
    error "Required: $1 is not installed."
    echo "  Install it from: $2"
    return 1
  fi
}

preflight() {
  log "Running preflight checks..."
  local missing=0

  check_dependency "docker" "https://docs.docker.com/get-docker/" || missing=1
  check_dependency "docker" "https://docs.docker.com/get-docker/" || missing=1

  if ! docker compose version &>/dev/null && ! docker-compose version &>/dev/null; then
    error "Required: docker compose (v2) or docker-compose is not available."
    echo "  Install it from: https://docs.docker.com/compose/install/"
    missing=1
  fi

  if ! docker info &>/dev/null; then
    error "Docker daemon is not running. Start Docker Desktop or the Docker service."
    missing=1
  fi

  if [ "$missing" -eq 1 ]; then
    echo ""
    error "Fix the issues above and re-run this script."
    exit 1
  fi

  log "All preflight checks passed."
}

compose_cmd() {
  if docker compose version &>/dev/null; then
    echo "docker compose"
  else
    echo "docker-compose"
  fi
}

install_flowpulse() {
  banner

  preflight

  if [ -d "$INSTALL_DIR/.git" ]; then
    log "Existing installation found at $INSTALL_DIR — pulling latest..."
    git -C "$INSTALL_DIR" pull --ff-only 2>/dev/null || true
  elif [ -d "$INSTALL_DIR" ] && [ -f "$INSTALL_DIR/docker-compose.quickstart.yml" ]; then
    log "Existing installation found at $INSTALL_DIR (no git). Updating in place..."
  else
    log "Cloning FlowPulse to $INSTALL_DIR..."
    if command -v git &>/dev/null; then
      git clone --depth 1 "$REPO_URL" "$INSTALL_DIR"
    else
      warn "git not found — downloading as archive..."
      mkdir -p "$INSTALL_DIR"
      curl -sL "$REPO_URL/archive/refs/heads/main.tar.gz" | tar xz --strip-components=1 -C "$INSTALL_DIR"
    fi
  fi

  cd "$INSTALL_DIR"

  if [ ! -f .env ]; then
    log "Creating .env with default configuration..."
    cat > .env <<ENVEOF
# FlowPulse Configuration
# Edit these values to customize your installation

# Version (use 'latest' or a specific tag like 'v0.1.0')
FLOWPULSE_VERSION=${FLOWPULSE_VERSION}

# Port mappings (change if defaults conflict)
WEB_PORT=3000
API_PORT=8080
AGGREGATOR_HTTP_PORT=9092
AGGREGATOR_GRPC_PORT=9091
CLICKHOUSE_HTTP_PORT=8123
CLICKHOUSE_NATIVE_PORT=9000
NATS_PORT=4222
NATS_MONITOR_PORT=8222
REDIS_PORT=6379

# Security (generate a strong secret for production)
JWT_SECRET=flowpulse-quickstart-$(openssl rand -hex 16 2>/dev/null || date +%s)
ENVEOF
  fi

  local COMPOSE
  COMPOSE=$(compose_cmd)

  log "Building and starting FlowPulse..."
  $COMPOSE -f docker-compose.quickstart.yml build 2>&1 | tail -5
  $COMPOSE -f docker-compose.quickstart.yml up -d

  echo ""
  log "Waiting for services to become healthy..."
  local retries=0
  local max_retries=60
  while [ $retries -lt $max_retries ]; do
    if $COMPOSE -f docker-compose.quickstart.yml ps --format json 2>/dev/null | grep -q '"Health":"healthy"' 2>/dev/null; then
      break
    fi
    # Fallback: check if web container is running
    if $COMPOSE -f docker-compose.quickstart.yml ps 2>/dev/null | grep -q "web.*running" 2>/dev/null; then
      break
    fi
    retries=$((retries + 1))
    sleep 2
  done

  echo ""
  echo -e "${GREEN}${BOLD}  ✓ FlowPulse is running!${NC}"
  echo ""
  echo -e "  ${BOLD}Dashboard:${NC}        http://localhost:${WEB_PORT:-3000}"
  echo -e "  ${BOLD}API Server:${NC}       http://localhost:${API_PORT:-8080}/healthz"
  echo -e "  ${BOLD}API Docs:${NC}         http://localhost:${API_PORT:-8080}/api/v1/"
  echo -e "  ${BOLD}ClickHouse:${NC}       http://localhost:${CLICKHOUSE_HTTP_PORT:-8123}"
  echo -e "  ${BOLD}NATS Monitor:${NC}     http://localhost:${NATS_MONITOR_PORT:-8222}"
  echo ""
  echo -e "  ${BOLD}To start with demo data:${NC}"
  echo -e "    $COMPOSE -f docker-compose.quickstart.yml --profile demo up -d"
  echo ""
  echo -e "  ${BOLD}To view logs:${NC}"
  echo -e "    $COMPOSE -f docker-compose.quickstart.yml logs -f"
  echo ""
  echo -e "  ${BOLD}To stop:${NC}"
  echo -e "    $COMPOSE -f docker-compose.quickstart.yml down"
  echo ""
  echo -e "  ${BOLD}To stop and remove data:${NC}"
  echo -e "    $COMPOSE -f docker-compose.quickstart.yml down -v"
  echo ""
}

install_flowpulse
