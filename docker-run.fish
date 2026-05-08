#!/usr/bin/env fish

set REGISTRY "registry.digitalocean.com/rzilient-do-containers"
set IMAGE    "vpn-portal"
set TAG      (git rev-parse --short HEAD 2>/dev/null; or echo "local")

# ─── Parse args ───────────────────────────────────────────────────────────────
set MODE "all"   # all | build | push | run

if test (count $argv) -gt 0
  set MODE $argv[1]
end

# ─── Helpers ──────────────────────────────────────────────────────────────────
function log_step
  echo -e "\033[38;5;34m  ──── $argv\033[0m"
end

function log_ok
  echo -e "\033[38;5;46m  ✓ $argv\033[0m"
end

function log_err
  echo -e "\033[0;31m  ✗ $argv\033[0m"
end

function log_info
  echo -e "\033[38;5;40m  · $argv\033[0m"
end

# ─── Check .env ───────────────────────────────────────────────────────────────
function check_env
  if not test -f .env
    log_err ".env file not found — copy .env.example and fill in your credentials"
    exit 1
  end
end

# ─── Build ────────────────────────────────────────────────────────────────────
function do_build
  log_step "Building Docker image"
  docker build \
    --label "git.sha=$TAG" \
    -t $REGISTRY/$IMAGE:latest \
    -t $REGISTRY/$IMAGE:$TAG \
    .
  log_ok "Built $REGISTRY/$IMAGE:latest ($TAG)"
end

# ─── Push ─────────────────────────────────────────────────────────────────────
function do_push
  log_step "Fetching DO API token from 1Password"
  set DO_API_TOKEN (op read "op://Engineering/API_Production/DO_API_TOKEN")
  log_ok "Token fetched"

  log_step "Authenticating with DO Container Registry"
  doctl auth init --access-token $DO_API_TOKEN
  doctl registry login
  log_ok "Authenticated"

  log_step "Pushing to registry"
  docker push $REGISTRY/$IMAGE:latest
  docker push $REGISTRY/$IMAGE:$TAG
  log_ok "Pushed $REGISTRY/$IMAGE:latest"
  log_ok "Pushed $REGISTRY/$IMAGE:$TAG"
end

# ─── Run locally ──────────────────────────────────────────────────────────────
function do_run
  check_env

  # Stop existing container if running
  if docker ps -q --filter name=vpn-portal-dev | grep -q .
    log_step "Stopping existing container"
    docker stop vpn-portal-dev
    docker rm vpn-portal-dev
  end

  log_step "Starting vpn-portal locally"
  docker run -d \
    --name vpn-portal-dev \
    -p 8080:8080 \
    --cap-add NET_ADMIN \
    --env-file .env \
    $REGISTRY/$IMAGE:latest

  log_ok "Running at http://localhost:8080"
  log_info "Logs:  docker logs -f vpn-portal-dev"
  log_info "Stop:  docker stop vpn-portal-dev"
end

# ─── Main ─────────────────────────────────────────────────────────────────────
echo ""
echo -e "\033[38;5;28m  ░▒▓ vpn-portal — build & run ▓▒░\033[0m"
echo ""
log_info "Registry: $REGISTRY/$IMAGE"
log_info "Tag:      $TAG"
log_info "Mode:     $MODE"
echo ""

switch $MODE
  case "build"
    do_build
  case "push"
    do_build
    do_push
  case "run"
    do_run
  case "all"
    do_build
    do_push
    do_run
  case "*"
    log_err "Unknown mode: $MODE"
    echo ""
    echo "  Usage: ./docker-run.fish [mode]"
    echo ""
    echo "  Modes:"
    echo "    build   Build image only"
    echo "    push    Build and push to registry"
    echo "    run     Run locally (uses existing image)"
    echo "    all     Build, push and run locally (default)"
    echo ""
    exit 1
end