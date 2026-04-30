#!/bin/bash
set -e

CONF_FILE="${1:-rzilient-vpn.conf}"
CONNECTION_NAME="rzilient-vpn"

if [ ! -f "$CONF_FILE" ]; then
  echo "Usage: $0 <path-to-conf-file>"
  echo "Example: $0 ~/Downloads/rzilient-vpn.conf"
  exit 1
fi

echo "==> Installing WireGuard config"
sudo cp "$CONF_FILE" /etc/wireguard/${CONNECTION_NAME}.conf
sudo chmod 600 /etc/wireguard/${CONNECTION_NAME}.conf

echo "==> Fixing resolvconf if needed"
if command -v resolvconf &> /dev/null; then
  sudo resolvconf -u 2>/dev/null || true
fi

echo "==> Connecting"
sudo wg-quick up ${CONNECTION_NAME}

echo ""
echo "✓ Connected. Verify with: curl ifconfig.me"
echo "  To disconnect: sudo wg-quick down ${CONNECTION_NAME}"
echo "  To auto-start on boot: sudo systemctl enable --now wg-quick@${CONNECTION_NAME}"