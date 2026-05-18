#!/bin/bash
set -e

# ─── Colors ───────────────────────────────────────────────────────────────────
DARK_GREEN='\033[38;5;22m'
GREEN='\033[38;5;28m'
BRIGHT_GREEN='\033[38;5;34m'
LIME_GREEN='\033[38;5;40m'
PALE_GREEN='\033[38;5;82m'
COLOUR_END='\033[0m'

REGISTRY="registry.digitalocean.com/rzilient-do-containers"
IMAGE="vpn-portal"

# ─── Logo ─────────────────────────────────────────────────────────────────────
function header() {
  echo -e "
  \033[38;5;22m                        .__.__  .__               __       __                .__\033[0m
  \033[38;5;28m          ______________|__|  | |__| ____   _____/  |_   _/  |_  ____   ____ |  |__\033[0m
  \033[38;5;34m          \_  __ \___   /  |  | |  |/ __\\ /    \\   __\\  \\   __\\/ __ \\_/ ___\\|  |  \\\033[0m
  \033[38;5;40m           |  | \\//    /|  |  |_|  \\  ___/|   |  \\  |     |  | \\  ___/\\  \\___|   Y  \\\033[0m
  \033[38;5;46m           |__|  /_____ \\__|____/__|\\___|  >___|  /__|     |__|  \\___  >\\___  >___|  /\033[0m
  \033[38;5;82m   ▀▀▀▀▀▀▀             \\/               \\/     \\/                   \\/     \\/     \\/\033[0m
  \033[38;5;22m   https://rzilient.tech | eduard@rzilient.club\033[0m
"
}

# ─── Helpers ──────────────────────────────────────────────────────────────────
function log_step() { echo -e "\033[38;5;34m  ──── $1\033[0m"; }
function log_ok()   { echo -e "\033[38;5;46m  ✓ $1\033[0m"; }
function log_skip() { echo -e "\033[38;5;82m  ↷ $1\033[0m"; }
function log_err()  { echo -e "\033[0;31m  ✗ $1\033[0m"; }
function log_info() { echo -e "\033[38;5;40m  · $1\033[0m"; }

# ─── Usage ────────────────────────────────────────────────────────────────────
usage() {
  header
  echo -e "\033[38;5;28m        ░▒▓ VPN Deploy Tool ▓▒░\033[0m"
  echo ""
  echo -e "\033[38;5;46m  Usage: $0 <command> [options]\033[0m"
  echo ""
  echo -e "\033[38;5;40m  Commands:\033[0m"
  echo "    deploy            Fresh installation on a new server"
  echo "    update            Update vpn-portal on an existing server"
  echo "    init              Push vpn-portal.service secrets and restart"
  echo ""
  echo -e "\033[38;5;40m  Deploy options:\033[0m"
  echo "    -h, --host        Server IP or hostname          (required)"
  echo "    -u, --user        SSH user                       (default: root)"
  echo "    -p, --port        SSH port                       (default: 22)"
  echo "    -d, --domain      VPN portal domain              (required)"
  echo "    -e, --email       Let's Encrypt contact email    (required)"
  echo "    -s, --subnet      WireGuard subnet prefix        (default: 10.8.0)"
  echo "        --no-docker   Build from source instead of Docker image"
  echo ""
  echo -e "\033[38;5;40m  Update options:\033[0m"
  echo "    -h, --host        Server IP or hostname          (required)"
  echo "    -u, --user        SSH user                       (default: root)"
  echo "    -p, --port        SSH port                       (default: 22)"
  echo "        --no-docker   Rebuild from source instead of pulling image"
  echo ""
  echo -e "\033[38;5;40m  Init options:\033[0m"
  echo "    -h, --host        Server IP or hostname          (required)"
  echo "    -u, --user        SSH user                       (default: root)"
  echo "    -p, --port        SSH port                       (default: 22)"
  echo ""
  echo -e "\033[38;5;40m  Examples:\033[0m"
  echo "    $0 deploy --host 1.2.3.4 --domain vpn.example.com --email admin@example.com"
  echo "    $0 deploy --host 1.2.3.4 --domain vpn-fr.example.com --email admin@example.com --subnet 10.9.0 --no-docker"
  echo "    $0 update --host 1.2.3.4"
  echo "    $0 update --host 1.2.3.4 --no-docker"
  echo "    $0 init --host 1.2.3.4"
  echo ""
  exit 1
}

# ─── Defaults ─────────────────────────────────────────────────────────────────
SSH_USER="root"
SSH_PORT="22"
DOMAIN=""
EMAIL=""
SUBNET="10.8.0"
HOST=""
NO_DOCKER=false

# ─── Parse command ────────────────────────────────────────────────────────────
COMMAND="${1:-}"
if [[ "$COMMAND" != "deploy" && "$COMMAND" != "update" && "$COMMAND" != "init" ]]; then
  usage
fi
shift

# ─── Parse args ───────────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--host)      HOST="$2";      shift 2 ;;
    -u|--user)      SSH_USER="$2";  shift 2 ;;
    -p|--port)      SSH_PORT="$2";  shift 2 ;;
    -d|--domain)    DOMAIN="$2";    shift 2 ;;
    -e|--email)     EMAIL="$2";     shift 2 ;;
    -s|--subnet)    SUBNET="$2";    shift 2 ;;
    --no-docker)    NO_DOCKER=true; shift ;;
    --help)         usage ;;
    *) log_err "Unknown option: $1"; usage ;;
  esac
done

# ─── Validate args ────────────────────────────────────────────────────────────
ERRORS=0
if [ -z "$HOST" ]; then log_err "--host is required"; ERRORS=1; fi

if [[ "$COMMAND" == "deploy" ]]; then
  if [ -z "$DOMAIN" ]; then log_err "--domain is required"; ERRORS=1; fi
  if [ -z "$EMAIL" ];  then log_err "--email is required";  ERRORS=1; fi
fi

[ $ERRORS -ne 0 ] && echo "" && usage

SSH_TARGET="$SSH_USER@$HOST"
LOCAL_DIR="$(dirname "$0")"

# ══════════════════════════════════════════════════════════════════════════════
# UPDATE
# ══════════════════════════════════════════════════════════════════════════════
if [[ "$COMMAND" == "update" ]]; then
  echo ""
  log_info "Host:    $SSH_TARGET"
  if [[ "$NO_DOCKER" == true ]]; then
    log_info "Mode:    source (--no-docker)"
  else
    log_info "Mode:    docker"
    log_info "Image:   $REGISTRY/$IMAGE:latest"
  fi
  echo ""

  if [[ "$NO_DOCKER" == true ]]; then
    log_step "Copying files to server"
    scp "$LOCAL_DIR/main.go"       "$SSH_TARGET:/opt/vpn-portal/"
    scp "$LOCAL_DIR/go.mod"        "$SSH_TARGET:/opt/vpn-portal/"
    scp "$LOCAL_DIR/manifest.json" "$SSH_TARGET:/opt/vpn-portal/"
    scp -r "$LOCAL_DIR/static"     "$SSH_TARGET:/opt/vpn-portal/"
    scp -r "$LOCAL_DIR/templates"  "$SSH_TARGET:/opt/vpn-portal/"
    log_ok "Files copied"
    echo ""

    log_step "Building and restarting on $HOST"
    ssh "$SSH_TARGET" 'set -e
cd /opt/vpn-portal
go mod tidy
go build -o vpn-portal .
systemctl restart vpn-portal
systemctl status vpn-portal --no-pager'

    echo ""
    log_ok "vpn-portal updated on $HOST"
    log_info "Logs: ssh $SSH_TARGET journalctl -u vpn-portal -f"
  else
    log_step "Pulling latest image and restarting on $HOST"
    ssh "$SSH_TARGET" "
      docker pull $REGISTRY/$IMAGE:latest &&
      docker restart vpn-portal &&
      docker ps --filter name=vpn-portal --format 'table {{.Status}}'
    "

    echo ""
    log_ok "vpn-portal updated on $HOST"
    log_info "Logs: ssh $SSH_TARGET docker logs -f vpn-portal"
  fi

  echo ""
  exit 0
fi

# ══════════════════════════════════════════════════════════════════════════════
# INIT — inject secrets and restart
# ══════════════════════════════════════════════════════════════════════════════
if [[ "$COMMAND" == "init" ]]; then
  echo ""
  log_info "Host:    $SSH_TARGET"
  echo ""

  GOOGLE_CLIENT_ID=$(grep "^Environment=GOOGLE_CLIENT_ID=" "$LOCAL_DIR/vpn-portal.service" | cut -d= -f3-)
  GOOGLE_CLIENT_SECRET=$(grep "^Environment=GOOGLE_CLIENT_SECRET=" "$LOCAL_DIR/vpn-portal.service" | cut -d= -f3-)
  ALLOWED_DOMAINS=$(grep "^Environment=ALLOWED_DOMAINS=" "$LOCAL_DIR/vpn-portal.service" | cut -d= -f3-)
  ADMIN_TOKEN=$(grep "^Environment=ADMIN_TOKEN=" "$LOCAL_DIR/vpn-portal.service" | cut -d= -f3-)
  SMTP_HOST=$(grep "^Environment=SMTP_HOST=" "$LOCAL_DIR/vpn-portal.service" | cut -d= -f3-)
  SMTP_PORT=$(grep "^Environment=SMTP_PORT=" "$LOCAL_DIR/vpn-portal.service" | cut -d= -f3-)
  SMTP_USERNAME=$(grep "^Environment=SMTP_USERNAME=" "$LOCAL_DIR/vpn-portal.service" | cut -d= -f3-)
  SMTP_PASSWORD=$(grep "^Environment=SMTP_PASSWORD=" "$LOCAL_DIR/vpn-portal.service" | cut -d= -f3-)
  SMTP_FROM=$(grep "^Environment=SMTP_FROM=" "$LOCAL_DIR/vpn-portal.service" | cut -d= -f3-)

  ERRORS=0
  if [[ "$GOOGLE_CLIENT_ID" == *"<"* || -z "$GOOGLE_CLIENT_ID" ]];         then log_err "GOOGLE_CLIENT_ID not set in vpn-portal.service";     ERRORS=1; fi
  if [[ "$GOOGLE_CLIENT_SECRET" == *"<"* || -z "$GOOGLE_CLIENT_SECRET" ]]; then log_err "GOOGLE_CLIENT_SECRET not set in vpn-portal.service"; ERRORS=1; fi
  if [[ "$ALLOWED_DOMAINS" == *"<"* || -z "$ALLOWED_DOMAINS" ]];           then log_err "ALLOWED_DOMAINS not set in vpn-portal.service";       ERRORS=1; fi
  if [[ "$ADMIN_TOKEN" == *"<"* || -z "$ADMIN_TOKEN" ]];                   then log_err "ADMIN_TOKEN not set in vpn-portal.service";           ERRORS=1; fi
  if [[ -z "$SMTP_USERNAME" ]]; then log_err "SMTP_USERNAME not set in vpn-portal.service"; ERRORS=1; fi
  if [[ -z "$SMTP_PASSWORD" ]]; then log_err "SMTP_PASSWORD not set in vpn-portal.service"; ERRORS=1; fi
  [ $ERRORS -ne 0 ] && echo "" && echo "  Edit vpn-portal.service locally and set the required values first." && echo "" && exit 1

  log_step "Injecting secrets on $HOST"
  ssh "$SSH_TARGET" bash << ENDSSH
set -e
# Update systemd service file
sed -i "s|Environment=GOOGLE_CLIENT_ID=.*|Environment=GOOGLE_CLIENT_ID=${GOOGLE_CLIENT_ID}|"             /etc/systemd/system/vpn-portal.service
sed -i "s|Environment=GOOGLE_CLIENT_SECRET=.*|Environment=GOOGLE_CLIENT_SECRET=${GOOGLE_CLIENT_SECRET}|" /etc/systemd/system/vpn-portal.service
sed -i "s|Environment=ALLOWED_DOMAINS=.*|Environment=ALLOWED_DOMAINS=${ALLOWED_DOMAINS}|"                 /etc/systemd/system/vpn-portal.service
sed -i "s|Environment=ADMIN_TOKEN=.*|Environment=ADMIN_TOKEN=${ADMIN_TOKEN}|"                             /etc/systemd/system/vpn-portal.service

# Update Docker env file
if [ -f /etc/vpn-portal.env ]; then
  sed -i "s|GOOGLE_CLIENT_ID=.*|GOOGLE_CLIENT_ID=${GOOGLE_CLIENT_ID}|"             /etc/vpn-portal.env
  sed -i "s|GOOGLE_CLIENT_SECRET=.*|GOOGLE_CLIENT_SECRET=${GOOGLE_CLIENT_SECRET}|" /etc/vpn-portal.env
  sed -i "s|ALLOWED_DOMAINS=.*|ALLOWED_DOMAINS=${ALLOWED_DOMAINS}|"                 /etc/vpn-portal.env
  sed -i "s|ADMIN_TOKEN=.*|ADMIN_TOKEN=${ADMIN_TOKEN}|"                             /etc/vpn-portal.env

  # SMTP — update if exists, append if missing
  for key in SMTP_HOST SMTP_PORT SMTP_USERNAME SMTP_PASSWORD SMTP_FROM; do
    val="${!key}"
    if [ -n "$val" ]; then
      if grep -q "^${key}=" /etc/vpn-portal.env; then
        sed -i "s|${key}=.*|${key}=${val}|" /etc/vpn-portal.env
      else
        echo "${key}=${val}" >> /etc/vpn-portal.env
      fi
    fi
  done
fi

# Recreate docker container to pick up new env file
systemctl daemon-reload
if docker ps -aq --filter name=vpn-portal | grep -q .; then
  docker stop vpn-portal
  docker rm vpn-portal
  docker run -d \
    --name vpn-portal \
    --restart unless-stopped \
    --network host \
    --cap-add NET_ADMIN \
    --env-file /etc/vpn-portal.env \
    -v /etc/wireguard:/etc/wireguard \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v /usr/local/bin/vpn-update:/usr/local/bin/vpn-update \
    -v /root/.docker:/root/.docker \
    -v /root/.config/doctl:/root/.config/doctl:ro \
    -v /usr/local/bin/doctl:/usr/local/bin/doctl:ro \
    ${REGISTRY}/${IMAGE}:latest
  echo "    container recreated with new secrets"
else
  systemctl restart vpn-portal 2>/dev/null && echo "    systemd service restarted" || echo "    nothing to restart"
fi
ENDSSH

  log_ok "Secrets injected on $HOST"
  echo ""
  exit 0
fi

# ══════════════════════════════════════════════════════════════════════════════
# DEPLOY — fresh installation
# ══════════════════════════════════════════════════════════════════════════════
echo ""
log_info "Host:    $SSH_TARGET"
log_info "Domain:  $DOMAIN"
log_info "Subnet:  ${SUBNET}.0/24"
log_info "Email:   $EMAIL"
if [[ "$NO_DOCKER" == true ]]; then
  log_info "Mode:    source (--no-docker)"
else
  log_info "Mode:    docker"
  log_info "Image:   $REGISTRY/$IMAGE:latest"
fi
echo ""

# ─── Copy files ───────────────────────────────────────────────────────────────
log_step "Copying config files to server"
ssh "$SSH_TARGET" "mkdir -p /tmp/vpn-portal"
scp -q "$LOCAL_DIR/nginx.conf"         "$SSH_TARGET:/tmp/vpn-portal/"
scp -q "$LOCAL_DIR/vpn-portal.service" "$SSH_TARGET:/tmp/vpn-portal/"

if [[ "$NO_DOCKER" == true ]]; then
  scp -q "$LOCAL_DIR/main.go"            "$SSH_TARGET:/tmp/vpn-portal/"
  scp -q "$LOCAL_DIR/go.mod"             "$SSH_TARGET:/tmp/vpn-portal/"
  scp -q "$LOCAL_DIR/manifest.json"      "$SSH_TARGET:/tmp/vpn-portal/"
  scp -q -r "$LOCAL_DIR/static"          "$SSH_TARGET:/tmp/vpn-portal/"
  scp -q -r "$LOCAL_DIR/templates"       "$SSH_TARGET:/tmp/vpn-portal/"
fi
log_ok "Files copied"
echo ""

# ─── Remote deploy ────────────────────────────────────────────────────────────
log_step "Running remote deployment on $HOST"
echo ""

# Read DO token locally before opening SSH session
if [[ "$NO_DOCKER" == false ]]; then
  if [ -z "$DO_API_TOKEN" ]; then
    log_err "DO_API_TOKEN is not set — required for Docker registry auth"
    log_info "Set it with: export DO_API_TOKEN=<your_token>"
    exit 1
  fi
fi

ssh "$SSH_TARGET" bash << REMOTE
set -e
export DEBIAN_FRONTEND=noninteractive
DO_API_TOKEN="${DO_API_TOKEN}"
REGISTRY="${REGISTRY}"
IMAGE="${IMAGE}"

echo "==> Installing dependencies"
apt update -q
$(if [[ "$NO_DOCKER" == true ]]; then
  echo "apt install -y nginx certbot python3-certbot-nginx wireguard-tools resolvconf iptables-persistent wget tar > /dev/null"
  echo "wget -q https://go.dev/dl/go1.25.0.linux-amd64.tar.gz -O /tmp/go.tar.gz"
  echo "rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tar.gz && rm /tmp/go.tar.gz"
  echo "export PATH=\\$PATH:/usr/local/go/bin"
  echo "echo 'export PATH=\\$PATH:/usr/local/go/bin' >> /etc/profile.d/go.sh"
else
  echo "apt install -y nginx certbot python3-certbot-nginx wireguard-tools resolvconf iptables-persistent docker.io > /dev/null"
fi)
echo "    done"

echo "==> Setting up WireGuard"
mkdir -p /etc/wireguard/peers

if [ ! -f /etc/wireguard/server_private.key ]; then
  wg genkey | tee /etc/wireguard/server_private.key | wg pubkey > /etc/wireguard/server_public.key
  chmod 600 /etc/wireguard/server_private.key
  echo "    keys generated"
else
  echo "    keys already exist — skipping"
fi

echo 'net.ipv4.ip_forward=1' > /etc/sysctl.d/99-wireguard.conf
sysctl -qp /etc/sysctl.d/99-wireguard.conf

if [ ! -f /etc/wireguard/wg0.conf ]; then
  NET_IF=\$(ip route show default | awk '/default/ {print \$5}' | head -1)
  echo "    detected network interface: \${NET_IF}"
  cat > /etc/wireguard/wg0.conf << WGEOF
[Interface]
PrivateKey = \$(cat /etc/wireguard/server_private.key)
Address = ${SUBNET}.1/24
ListenPort = 51820
PostUp = iptables -A FORWARD -i wg0 -j ACCEPT; iptables -t nat -A POSTROUTING -o \${NET_IF} -j MASQUERADE
PostDown = iptables -D FORWARD -i wg0 -j ACCEPT; iptables -t nat -D POSTROUTING -o \${NET_IF} -j MASQUERADE
WGEOF
  echo "    config created (interface: \${NET_IF})"
else
  echo "    config already exists — skipping"
fi

systemctl enable --now wg-quick@wg0 2>/dev/null || true
echo "    running on port 51820"

echo "==> Setting up UDP 443 fallback"
NET_IF=\$(ip route show default | awk '/default/ {print \$5}' | head -1)
if ! iptables -t nat -C PREROUTING -i \${NET_IF} -p udp --dport 443 -j REDIRECT --to-port 51820 2>/dev/null; then
  iptables -t nat -A PREROUTING -i \${NET_IF} -p udp --dport 443 -j REDIRECT --to-port 51820
  netfilter-persistent save > /dev/null
  echo "    UDP 443 -> 51820 redirect added (interface: \${NET_IF})"
else
  echo "    UDP 443 -> 51820 already exists — skipping"
fi

$(if [[ "$NO_DOCKER" == true ]]; then
cat << 'NODOCKEREOF'
echo "==> Building vpn-portal"
export PATH=$PATH:/usr/local/go/bin
mkdir -p /opt/vpn-portal
cd /opt/vpn-portal
cp /tmp/vpn-portal/main.go .
cp /tmp/vpn-portal/go.mod .
cp /tmp/vpn-portal/manifest.json .
cp -r /tmp/vpn-portal/static .
cp -r /tmp/vpn-portal/templates .
go mod tidy
go build -o vpn-portal .
echo "    built"

echo "==> Installing systemd service"
cp /tmp/vpn-portal/vpn-portal.service /etc/systemd/system/

SERVER_PUBLIC_KEY=$(cat /etc/wireguard/server_public.key)
SERVER_IP=$(curl -s -4 ifconfig.me)

sed -i "s|WG_SERVER_PUBLIC_KEY=.*|WG_SERVER_PUBLIC_KEY=${SERVER_PUBLIC_KEY}|" /etc/systemd/system/vpn-portal.service
sed -i "s|WG_SERVER_ENDPOINT=.*|WG_SERVER_ENDPOINT=${SERVER_IP}|"             /etc/systemd/system/vpn-portal.service
sed -i "s|VPN_SUBNET=.*|VPN_SUBNET=SUBNET_PLACEHOLDER|"                       /etc/systemd/system/vpn-portal.service
sed -i "s|BASE_URL=.*|BASE_URL=https://DOMAIN_PLACEHOLDER|"                   /etc/systemd/system/vpn-portal.service
systemctl daemon-reload
systemctl enable vpn-portal
systemctl start vpn-portal
echo "    running"
NODOCKEREOF
else
cat << 'DOCKEREOF'
echo "==> Installing doctl and authenticating with DO Container Registry"
cd /tmp
wget -q https://github.com/digitalocean/doctl/releases/download/v1.104.0/doctl-1.104.0-linux-amd64.tar.gz
tar -xzf doctl-1.104.0-linux-amd64.tar.gz
mv doctl /usr/local/bin/
doctl auth init --access-token ${DO_API_TOKEN}
doctl registry login
echo "    authenticated"

echo "==> Pulling vpn-portal image"
docker pull ${REGISTRY}/${IMAGE}:latest
echo "    pulled"

echo "==> Creating env file"
SERVER_PUBLIC_KEY=$(cat /etc/wireguard/server_public.key)
SERVER_IP=$(curl -s -4 ifconfig.me)

cat > /etc/vpn-portal.env << EOF
WG_SERVER_PUBLIC_KEY=${SERVER_PUBLIC_KEY}
WG_SERVER_ENDPOINT=${SERVER_IP}
VPN_SUBNET=SUBNET_PLACEHOLDER
BASE_URL=https://DOMAIN_PLACEHOLDER
WG_INTERFACE=wg0
STATE_FILE=/etc/wireguard/peers/state.json
PORT=8080
GOOGLE_CLIENT_ID=<your_google_client_id>
GOOGLE_CLIENT_SECRET=<your_google_client_secret>
ALLOWED_DOMAINS=<your_allowed_domains>
ADMIN_TOKEN=<your_secure_admin_token>
SMTP_HOST=smtp.eu.mailgun.org
SMTP_PORT=587
SMTP_USERNAME=<your_smtp_username>
SMTP_PASSWORD=<your_smtp_password>
SMTP_FROM=<your_smtp_from>
EOF
chmod 600 /etc/vpn-portal.env

echo "==> Starting vpn-portal container"
docker stop vpn-portal 2>/dev/null || true
docker rm vpn-portal 2>/dev/null || true
docker run -d \
  --name vpn-portal \
  --restart unless-stopped \
  --network host \
  --cap-add NET_ADMIN \
  --env-file /etc/vpn-portal.env \
  -v /etc/wireguard:/etc/wireguard \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /usr/local/bin/vpn-update:/usr/local/bin/vpn-update \
  ${REGISTRY}/${IMAGE}:latest
echo "    running"

echo "==> Installing vpn-update script"
cat > /usr/local/bin/vpn-update << 'UPDATEEOF'
#!/bin/bash
set -e
REGISTRY="registry.digitalocean.com/rzilient-do-containers"
IMAGE="vpn-portal"
doctl registry login
docker pull $REGISTRY/$IMAGE:latest
docker stop vpn-portal
docker rm vpn-portal
docker run -d \
  --name vpn-portal \
  --restart unless-stopped \
  --network host \
  --cap-add NET_ADMIN \
  --env-file /etc/vpn-portal.env \
  -v /etc/wireguard:/etc/wireguard \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /usr/local/bin/vpn-update:/usr/local/bin/vpn-update \
  -v /root/.docker:/root/.docker \
  -v /root/.config/doctl:/root/.config/doctl:ro \
  -v /usr/local/bin/doctl:/usr/local/bin/doctl:ro \
  $REGISTRY/$IMAGE:latest
echo "vpn-portal updated"
UPDATEEOF
chmod +x /usr/local/bin/vpn-update
echo "    installed"

echo "==> Installing systemd service (reference only)"
cp /tmp/vpn-portal/vpn-portal.service /etc/systemd/system/
SERVER_PUBLIC_KEY=$(cat /etc/wireguard/server_public.key)
SERVER_IP=$(curl -s -4 ifconfig.me)
sed -i "s|WG_SERVER_PUBLIC_KEY=.*|WG_SERVER_PUBLIC_KEY=${SERVER_PUBLIC_KEY}|" /etc/systemd/system/vpn-portal.service
sed -i "s|WG_SERVER_ENDPOINT=.*|WG_SERVER_ENDPOINT=${SERVER_IP}|"             /etc/systemd/system/vpn-portal.service
sed -i "s|VPN_SUBNET=.*|VPN_SUBNET=SUBNET_PLACEHOLDER|"                       /etc/systemd/system/vpn-portal.service
sed -i "s|BASE_URL=.*|BASE_URL=https://DOMAIN_PLACEHOLDER|"                   /etc/systemd/system/vpn-portal.service
systemctl daemon-reload
DOCKEREOF
fi)

echo "==> Installing nginx"
cp /tmp/vpn-portal/nginx.conf /etc/nginx/sites-available/vpn-portal
sed -i "0,/server_name/s|server_name .*;|server_name ${DOMAIN};|" /etc/nginx/sites-available/vpn-portal
ln -sf /etc/nginx/sites-available/vpn-portal /etc/nginx/sites-enabled/vpn-portal
rm -f /etc/nginx/sites-enabled/default
nginx -tq
systemctl start nginx 2>/dev/null
systemctl reload nginx
echo "    running"

echo "==> Obtaining SSL certificate"
certbot --nginx -d ${DOMAIN} --non-interactive --agree-tos -m ${EMAIL} -q
systemctl reload nginx
echo "    done"
REMOTE

# ─── Fix placeholders ─────────────────────────────────────────────────────────
ssh "$SSH_TARGET" bash << FIXEOF
set -e
for f in /etc/systemd/system/vpn-portal.service /etc/vpn-portal.env; do
  [ -f "\$f" ] && sed -i \
    -e "s|SUBNET_PLACEHOLDER|${SUBNET}|g" \
    -e "s|DOMAIN_PLACEHOLDER|${DOMAIN}|g" \
    -e "s|REGISTRY_PLACEHOLDER|${REGISTRY}|g" \
    -e "s|IMAGE_PLACEHOLDER|${IMAGE}|g" \
    "\$f"
done
systemctl daemon-reload 2>/dev/null || true
FIXEOF

# ─── Local summary ────────────────────────────────────────────────────────────
echo ""
header
echo -e "\033[38;5;28m        ░▒▓ Deployment Complete ▓▒░\033[0m"
echo ""
log_ok "Portal:  https://${DOMAIN}"
log_ok "Ports:   UDP 51820 + UDP 443 (fallback)"
if [[ "$NO_DOCKER" == true ]]; then
  log_ok "Mode:    source build"
else
  log_ok "Image:   $REGISTRY/$IMAGE:latest"
fi
echo ""
echo -e "\033[38;5;28m        ░▒▓ Next Steps ▓▒░\033[0m"
echo ""
log_info "1. Add OAuth redirect URI in Google Cloud Console:"
echo -e "\033[38;5;82m           https://${DOMAIN}/auth/callback\033[0m"
log_info "2. Push secrets:"
echo -e "\033[38;5;82m           $0 init --host $HOST\033[0m"
log_info "3. Add server IP to VPN_ALLOWED_IPS in DO app spec"
echo ""
echo -e "\033[38;5;28m        ░▒▓ Commands ▓▒░\033[0m"
echo ""
if [[ "$NO_DOCKER" == true ]]; then
  log_info "status:  ssh $SSH_TARGET systemctl status vpn-portal"
  log_info "logs:    ssh $SSH_TARGET journalctl -u vpn-portal -f"
  log_info "update:  $0 update --host $HOST --no-docker"
else
  log_info "status:  ssh $SSH_TARGET docker ps"
  log_info "logs:    ssh $SSH_TARGET docker logs -f vpn-portal"
  log_info "update:  $0 update --host $HOST"
fi
log_info "wg:      ssh $SSH_TARGET wg show"
echo ""