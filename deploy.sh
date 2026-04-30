#!/bin/bash
set -e

# ─── Usage ────────────────────────────────────────────────────────────────────
usage() {
  echo "Usage: $0 [options]"
  echo ""
  echo "Options:"
  echo "  -h, --host        Server IP or hostname (required)"
  echo "  -u, --user        SSH user (default: root)"
  echo "  -p, --port        SSH port (default: 22)"
  echo "  -d, --domain      VPN portal domain (required)"
  echo "  -e, --email       Let's Encrypt email (required)"
  echo "  -s, --subnet      WireGuard subnet prefix (default: 10.8.0)"
  echo "  -k, --ssh-key     Path to SSH private key (default: ~/.ssh/id_rsa)"
  echo "      --help        Show this help"
  echo ""
  echo "Example:"
  echo "  $0 --host 146.190.20.101"
  echo "  $0 --host 51.158.100.1 --domain vpn-fr.rzilient.tech --subnet 10.9.0"
  exit 1
}

# ─── Defaults ─────────────────────────────────────────────────────────────────
SSH_USER="root"
SSH_PORT="22"
SSH_KEY="$HOME/.ssh/id_rsa"
DOMAIN=""
EMAIL=""
SUBNET="10.8.0"
HOST=""

# ─── Parse args ───────────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--host)       HOST="$2";       shift 2 ;;
    -u|--user)       SSH_USER="$2";   shift 2 ;;
    -p|--port)       SSH_PORT="$2";   shift 2 ;;
    -d|--domain)     DOMAIN="$2";     shift 2 ;;
    -e|--email)      EMAIL="$2";      shift 2 ;;
    -s|--subnet)     SUBNET="$2";     shift 2 ;;
    -k|--ssh-key)    SSH_KEY="$2";    shift 2 ;;
    --help)          usage ;;
    *) echo "Unknown option: $1"; usage ;;
  esac
done

if [ -z "$HOST" ]; then
  echo "Error: --host is required"
  echo ""
  usage
fi

if [ -z "$DOMAIN" ]; then
  echo "Error: --domain is required"
  echo ""
  usage
fi

if [ -z "$EMAIL" ]; then
  echo "Error: --email is required"
  echo ""
  usage
fi

SSH_TARGET="$SSH_USER@$HOST"
SSH_OPTS="-p $SSH_PORT -i $SSH_KEY -o StrictHostKeyChecking=accept-new"
LOCAL_DIR="$(dirname "$0")"

echo ""
echo "════════════════════════════════════════"
echo "  rzilient VPN — Fresh Deployment"
echo "════════════════════════════════════════"
echo "  Host:    $SSH_TARGET:$SSH_PORT"
echo "  Domain:  $DOMAIN"
echo "  Subnet:  $SUBNET.0/24"
echo "  Email:   $EMAIL"
echo "════════════════════════════════════════"
echo ""

# ─── Copy files ───────────────────────────────────────────────────────────────
echo "==> Copying files to server"
ssh $SSH_OPTS $SSH_TARGET "mkdir -p /tmp/vpn-portal"
scp $SSH_OPTS "$LOCAL_DIR/main.go"              "$SSH_TARGET:/tmp/vpn-portal/"
scp $SSH_OPTS "$LOCAL_DIR/go.mod"               "$SSH_TARGET:/tmp/vpn-portal/"
scp $SSH_OPTS "$LOCAL_DIR/nginx.conf"           "$SSH_TARGET:/tmp/vpn-portal/"
scp $SSH_OPTS "$LOCAL_DIR/vpn-portal.service"   "$SSH_TARGET:/tmp/vpn-portal/"
scp $SSH_OPTS -r "$LOCAL_DIR/templates"         "$SSH_TARGET:/tmp/vpn-portal/"
echo "✓ Files copied"

# ─── Remote deploy ────────────────────────────────────────────────────────────
echo "==> Running remote deployment on $HOST"
ssh $SSH_OPTS $SSH_TARGET bash << REMOTE
set -e

echo "==> Installing dependencies"
apt update -q
apt install -y golang-go nginx certbot python3-certbot-nginx wireguard openresolv

echo "==> Setting up WireGuard"
mkdir -p /etc/wireguard/peers

if [ ! -f /etc/wireguard/server_private.key ]; then
  wg genkey | tee /etc/wireguard/server_private.key | wg pubkey > /etc/wireguard/server_public.key
  chmod 600 /etc/wireguard/server_private.key
  echo "✓ WireGuard keys generated"
fi

echo "net.ipv4.ip_forward=1" >> /etc/sysctl.conf
sysctl -p

cat > /etc/wireguard/wg0.conf << WGEOF
[Interface]
PrivateKey = \$(cat /etc/wireguard/server_private.key)
Address = ${SUBNET}.1/24
ListenPort = 51820
PostUp = iptables -A FORWARD -i wg0 -j ACCEPT; iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
PostDown = iptables -D FORWARD -i wg0 -j ACCEPT; iptables -t nat -D POSTROUTING -o eth0 -j MASQUERADE
WGEOF

systemctl enable --now wg-quick@wg0
echo "✓ WireGuard running on port 51820"

echo "==> Setting up UDP 443 fallback for strict firewalls"
apt install -y iptables-persistent
iptables -t nat -A PREROUTING -i eth0 -p udp --dport 443 -j REDIRECT --to-port 51820
netfilter-persistent save
echo "✓ UDP 443 → 51820 redirect active"

echo "==> Building vpn-portal"
mkdir -p /opt/vpn-portal
cd /opt/vpn-portal
cp /tmp/vpn-portal/main.go .
cp /tmp/vpn-portal/go.mod .
cp -r /tmp/vpn-portal/templates .
go mod tidy
go build -o vpn-portal .
echo "✓ vpn-portal built"

echo "==> Installing systemd service"
cp /tmp/vpn-portal/vpn-portal.service /etc/systemd/system/

SERVER_PUBLIC_KEY=\$(cat /etc/wireguard/server_public.key)
SERVER_IP=\$(curl -s ifconfig.me)

sed -i "s|WG_SERVER_PUBLIC_KEY=.*|WG_SERVER_PUBLIC_KEY=\${SERVER_PUBLIC_KEY}|" /etc/systemd/system/vpn-portal.service
sed -i "s|WG_SERVER_ENDPOINT=.*|WG_SERVER_ENDPOINT=\${SERVER_IP}|" /etc/systemd/system/vpn-portal.service
sed -i "s|VPN_SUBNET=.*|VPN_SUBNET=${SUBNET}|" /etc/systemd/system/vpn-portal.service
sed -i "s|BASE_URL=.*|BASE_URL=https://${DOMAIN}|" /etc/systemd/system/vpn-portal.service

systemctl daemon-reload
systemctl enable vpn-portal
echo "✓ systemd service installed"

echo "==> Installing nginx config"
cp /tmp/vpn-portal/nginx.conf /etc/nginx/sites-available/vpn-portal
sed -i "s|server_name .*|server_name ${DOMAIN};|" /etc/nginx/sites-available/vpn-portal
ln -sf /etc/nginx/sites-available/vpn-portal /etc/nginx/sites-enabled/vpn-portal
rm -f /etc/nginx/sites-enabled/default
nginx -t
systemctl start nginx
echo "✓ nginx running"

echo "==> Obtaining SSL certificate"
certbot --nginx -d ${DOMAIN} --non-interactive --agree-tos -m ${EMAIL}
systemctl reload nginx
echo "✓ SSL certificate obtained"

echo "==> Starting vpn-portal"
systemctl start vpn-portal

echo ""
echo "════════════════════════════════════════"
echo "✓ VPN portal deployed at https://${DOMAIN}"
echo "✓ WireGuard public key: \$(cat /etc/wireguard/server_public.key)"
echo "✓ WireGuard ports: UDP 51820 + UDP 443 (fallback)"
echo "✓ Server IP: \$(curl -s ifconfig.me)"
echo "════════════════════════════════════════"
echo ""
echo "Next steps:"
echo "  - Add https://${DOMAIN}/auth/callback to Google Cloud Console OAuth redirect URIs"
echo "  - Add server IP to VPN_ALLOWED_IPS in DO app spec"
echo "  - Check status:  systemctl status vpn-portal"
echo "  - Logs:          journalctl -u vpn-portal -f"
echo "  - WireGuard:     wg show"
REMOTE