#!/usr/bin/env fish

if test -z "$argv[1]"
  echo "Usage: update.fish <droplet-ip>"
  echo "Example: update.fish 146.190.20.101"
  exit 1
end

set DROPLET "root@$argv[1]"
set LOCAL_DIR (dirname (status filename))

echo "==> Copying files to droplet"
scp "$LOCAL_DIR/main.go"      "$DROPLET:/opt/vpn-portal/"
scp "$LOCAL_DIR/go.mod"       "$DROPLET:/opt/vpn-portal/"
scp "$LOCAL_DIR/manifest.json" "$DROPLET:/opt/vpn-portal/"
scp -r "$LOCAL_DIR/static"    "$DROPLET:/opt/vpn-portal/"
scp -r "$LOCAL_DIR/templates" "$DROPLET:/opt/vpn-portal/"

echo "==> Building and restarting"
ssh $DROPLET 'set -e
cd /opt/vpn-portal
go mod tidy
go build -o vpn-portal .
systemctl restart vpn-portal
systemctl status vpn-portal --no-pager'

echo ""
echo "✓ vpn-portal updated and restarted on $argv[1]"
echo "  Logs: ssh $DROPLET journalctl -u vpn-portal -f"