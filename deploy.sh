#!/bin/bash
set -e

# ─── Colors ───────────────────────────────────────────────────────────────────
DARK_GREEN='\033[38;5;22m'
GREEN='\033[38;5;28m'
BRIGHT_GREEN='\033[38;5;34m'
LIME_GREEN='\033[38;5;40m'
PALE_GREEN='\033[38;5;82m'
COLOUR_END='\033[0m'

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
  echo "    update            Update code on an existing server"
  echo ""
  echo -e "\033[38;5;40m  Deploy options:\033[0m"
  echo "    -h, --host        Server IP or hostname          (required)"
  echo "    -u, --user        SSH user                       (default: root)"
  echo "    -p, --port        SSH port                       (default: 22)"
  echo "    -d, --domain      VPN portal domain              (required)"
  echo "    -e, --email       Let's Encrypt contact email    (required)"
  echo "    -s, --subnet      WireGuard subnet prefix        (default: 10.8.0)"
  echo ""
  echo -e "\033[38;5;40m  Update options:\033[0m"
  echo "    -h, --host        Server IP or hostname          (required)"
  echo "    -u, --user        SSH user                       (default: root)"
  echo "    -p, --port        SSH port                       (default: 22)"
  echo ""
  echo -e "\033[38;5;40m  Examples:\033[0m"
  echo "    $0 deploy --host 1.2.3.4 --domain vpn.example.com --email admin@example.com"
  echo "    $0 deploy --host 1.2.3.4 --domain vpn-fr.example.com --email admin@example.com --subnet 10.9.0"
  echo "    $0 update --host 1.2.3.4"
  echo "    $0 update --host 1.2.3.4 --user ubuntu --ssh-key ~/.ssh/my_key"
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

# ─── Parse command ────────────────────────────────────────────────────────────
COMMAND="${1:-}"
if [[ "$COMMAND" != "deploy" && "$COMMAND" != "update" ]]; then
  usage
fi
shift

# ─── Parse args ───────────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--host)    HOST="$2";      shift 2 ;;
    -u|--user)    SSH_USER="$2";  shift 2 ;;
    -p|--port)    SSH_PORT="$2";  shift 2 ;;
    -d|--domain)  DOMAIN="$2";    shift 2 ;;
    -e|--email)   EMAIL="$2";     shift 2 ;;
    -s|--subnet)  SUBNET="$2";    shift 2 ;;
    --help)       usage ;;
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
  echo ""

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
  log_ok "vpn-portal updated and restarted on $HOST"
  log_info "Logs: ssh $SSH_TARGET journalctl -u vpn-portal -f"
  echo ""
  exit 0
fi


# ══════════════════════════════════════════════════════════════════════════════
# DEPLOY
# ══════════════════════════════════════════════════════════════════════════════
echo ""
log_info "Host:    $SSH_TARGET:$SSH_PORT"
log_info "Domain:  $DOMAIN"
log_info "Subnet:  ${SUBNET}.0/24"
log_info "Email:   $EMAIL"
echo ""

# ─── Copy files to server ─────────────────────────────────────────────────────
log_step "Copying files to server"
ssh "$SSH_TARGET" "mkdir -p /tmp/vpn-portal"
scp -q "$LOCAL_DIR/main.go"            "$SSH_TARGET:/tmp/vpn-portal/"
scp -q "$LOCAL_DIR/go.mod"             "$SSH_TARGET:/tmp/vpn-portal/"
scp -q "$LOCAL_DIR/nginx.conf"         "$SSH_TARGET:/tmp/vpn-portal/"
scp -q "$LOCAL_DIR/vpn-portal.service" "$SSH_TARGET:/tmp/vpn-portal/"
scp -q "$LOCAL_DIR/manifest.json"      "$SSH_TARGET:/tmp/vpn-portal/"
scp -q -r "$LOCAL_DIR/static"          "$SSH_TARGET:/tmp/vpn-portal/"
scp -q -r "$LOCAL_DIR/templates"       "$SSH_TARGET:/tmp/vpn-portal/"
log_ok "Files copied"
echo ""

# ─── Remote deploy ────────────────────────────────────────────────────────────
log_step "Running remote deployment on $HOST"
echo ""
ssh "$SSH_TARGET" bash << REMOTE
set -e

echo "==> Installing dependencies"
apt update -q
apt install -y golang-go nginx certbot python3-certbot-nginx wireguard openresolv iptables-persistent > /dev/null
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

grep -qxF 'net.ipv4.ip_forward=1' /etc/sysctl.conf || echo "net.ipv4.ip_forward=1" >> /etc/sysctl.conf
sysctl -qp

if [ ! -f /etc/wireguard/wg0.conf ]; then
  cat > /etc/wireguard/wg0.conf << WGEOF
[Interface]
PrivateKey = \$(cat /etc/wireguard/server_private.key)
Address = ${SUBNET}.1/24
ListenPort = 51820
PostUp = iptables -A FORWARD -i wg0 -j ACCEPT; iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
PostDown = iptables -D FORWARD -i wg0 -j ACCEPT; iptables -t nat -D POSTROUTING -o eth0 -j MASQUERADE
WGEOF
  echo "    config created"
else
  echo "    config already exists — skipping"
fi

systemctl enable --now wg-quick@wg0 2>/dev/null || true
echo "    running on port 51820"

echo "==> Setting up UDP 443 fallback"
if ! iptables -t nat -C PREROUTING -i eth0 -p udp --dport 443 -j REDIRECT --to-port 51820 2>/dev/null; then
  iptables -t nat -A PREROUTING -i eth0 -p udp --dport 443 -j REDIRECT --to-port 51820
  netfilter-persistent save > /dev/null
  echo "    UDP 443 -> 51820 redirect added"
else
  echo "    UDP 443 -> 51820 already exists — skipping"
fi

echo "==> Building vpn-portal"
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

SERVER_PUBLIC_KEY=\$(cat /etc/wireguard/server_public.key)
SERVER_IP=\$(curl -s ifconfig.me)

sed -i "s|WG_SERVER_PUBLIC_KEY=.*|WG_SERVER_PUBLIC_KEY=\${SERVER_PUBLIC_KEY}|" /etc/systemd/system/vpn-portal.service
sed -i "s|WG_SERVER_ENDPOINT=.*|WG_SERVER_ENDPOINT=\${SERVER_IP}|"             /etc/systemd/system/vpn-portal.service
sed -i "s|VPN_SUBNET=.*|VPN_SUBNET=${SUBNET}|"                                 /etc/systemd/system/vpn-portal.service
sed -i "s|BASE_URL=.*|BASE_URL=https://${DOMAIN}|"                             /etc/systemd/system/vpn-portal.service
systemctl daemon-reload
systemctl enable vpn-portal
echo "    installed"

echo "==> Installing nginx"
cp /tmp/vpn-portal/nginx.conf /etc/nginx/sites-available/vpn-portal
sed -i "0,/server_name/s|server_name .*;|server_name ${DOMAIN};|" /etc/nginx/sites-available/vpn-portal
ln -sf /etc/nginx/sites-available/vpn-portal /etc/nginx/sites-enabled/vpn-portal
rm -f /etc/nginx/sites-enabled/default
nginx -tq
systemctl start nginx 2>/dev/null || systemctl reload nginx
echo "    running"

echo "==> Obtaining SSL certificate"
certbot --nginx -d ${DOMAIN} --non-interactive --agree-tos -m ${EMAIL} -q
systemctl reload nginx
echo "    done"

echo "==> Starting vpn-portal"
systemctl start vpn-portal
sleep 1
systemctl is-active --quiet vpn-portal && echo "    running" || echo "    FAILED — check: journalctl -u vpn-portal"
REMOTE

# ─── Local summary ────────────────────────────────────────────────────────────
echo ""
header
echo -e "\033[38;5;28m        ░▒▓ Deployment Complete ▓▒░\033[0m"
echo ""
log_ok "Portal:  https://${DOMAIN}"
log_ok "Ports:   UDP 51820 + UDP 443 (fallback)"
echo ""
echo -e "\033[38;5;28m        ░▒▓ Next Steps ▓▒░\033[0m"
echo ""
log_info "1. Add OAuth redirect URI in Google Cloud Console:"
echo -e "\033[38;5;82m           https://${DOMAIN}/auth/callback\033[0m"
log_info "2. Add server IP to VPN_ALLOWED_IPS in DO app spec"
log_info "3. Set ADMIN_TOKEN in vpn-portal.service on the server"
echo ""
echo -e "\033[38;5;28m        ░▒▓ Commands ▓▒░\033[0m"
echo ""
log_info "status:  ssh $SSH_TARGET systemctl status vpn-portal"
log_info "logs:    ssh $SSH_TARGET journalctl -u vpn-portal -f"
log_info "wg:      ssh $SSH_TARGET wg show"
echo ""