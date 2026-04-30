#!/usr/bin/env fish

set CONF_FILE (test -n "$argv[1]"; and echo $argv[1]; or echo "rzilient-vpn.conf")
set CONNECTION_NAME "rzilient-vpn"

if not test -f $CONF_FILE
  echo "Usage: connect-vpn.fish <path-to-conf-file>"
  echo "Example: connect-vpn.fish ~/Downloads/rzilient-vpn.conf"
  exit 1
end

echo "==> Installing WireGuard config"
sudo cp $CONF_FILE /etc/wireguard/$CONNECTION_NAME.conf
sudo chmod 600 /etc/wireguard/$CONNECTION_NAME.conf

echo "==> Fixing resolvconf if needed"
if command -q resolvconf
  sudo resolvconf -u 2>/dev/null; or true
end

echo "==> Connecting"
sudo wg-quick up $CONNECTION_NAME

echo ""
echo "✓ Connected. Verify with: curl ifconfig.me"
echo "  To disconnect: sudo wg-quick down $CONNECTION_NAME"
echo "  To auto-start on boot: sudo systemctl enable --now wg-quick@$CONNECTION_NAME"