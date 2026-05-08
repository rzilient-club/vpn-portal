bash
#!/usr/bin/env bash
# ── Reset and redeploy a VPN instance from scratch ────────────────────────────
# Usage: ./test-deploy.sh <host> <domain> <subnet>
# Example: ./test-deploy.sh 51.158.75.75 vpn-fr.rzilient.tech 10.9.0

set -e

HOST="${1:-51.158.75.75}"
DOMAIN="${2:-vpn-fr.rzilient.tech}"
SUBNET="${3:-10.9.0}"
EMAIL="eduard@rzilient.club"
SCRIPT_DIR="$(dirname "$0")"

echo ""
echo -e "\033[38;5;34m  ──── Resetting server $HOST\033[0m"
echo ""

ssh "root@$HOST" "
set -e

# Stop and remove container
docker stop vpn-portal 2>/dev/null || true
docker rm vpn-portal 2>/dev/null || true

# Remove nginx config
rm -f /etc/nginx/sites-enabled/vpn-portal
rm -f /etc/nginx/sites-available/vpn-portal
rm -f /etc/nginx/sites-enabled/default

# Remove SSL certs
rm -rf /etc/letsencrypt/live/${DOMAIN}
rm -rf /etc/letsencrypt/renewal/${DOMAIN}.conf

# Remove WireGuard config and keys
rm -f /etc/wireguard/wg0.conf
rm -f /etc/wireguard/server_private.key
rm -f /etc/wireguard/server_public.key
systemctl stop wg-quick@wg0 2>/dev/null || true

# Remove iptables rule
NET_IF=\$(ip route show default | awk '/default/ {print \$5}' | head -1)
iptables -t nat -D PREROUTING -i \$NET_IF -p udp --dport 443 -j REDIRECT --to-port 51820 2>/dev/null || true

# Remove env file
rm -f /etc/vpn-portal.env

# Restart nginx clean
systemctl restart nginx

echo 'Server reset complete'
"

echo ""
echo -e "\033[38;5;46m  ✓ Server reset\033[0m"
echo ""

# Read DO token from 1Password
echo -e "\033[38;5;34m  ──── Fetching DO API token\033[0m"
export DO_API_TOKEN=$(op read "op://Engineering/API_Production/DO_API_TOKEN")
echo -e "\033[38;5;46m  ✓ Token fetched\033[0m"
echo ""

# Deploy
"$SCRIPT_DIR/deploy.sh" deploy \
  --host "$HOST" \
  --domain "$DOMAIN" \
  --email "$EMAIL" \
  --subnet "$SUBNET"

# Push secrets
"$SCRIPT_DIR/deploy.sh" init --host "$HOST"