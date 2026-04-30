#!/bin/bash
set -e

if [ -z "$1" ]; then
  echo "Usage: $0 <droplet-ip>"
  echo "Example: $0 146.190.20.101"
  exit 1
fi

DROPLET="root@$1"
LOCAL_DIR="$(dirname "$0")"

echo "==> Copying files to droplet"
scp "$LOCAL_DIR/main.go" "$DROPLET:/opt/vpn-portal/"
scp "$LOCAL_DIR/go.mod" "$DROPLET:/opt/vpn-portal/"
scp -r "$LOCAL_DIR/templates" "$DROPLET:/opt/vpn-portal/"

echo "==> Building and restarting"
ssh "$DROPLET" << 'REMOTE'
set -e
cd /opt/vpn-portal
go mod tidy
go build -o vpn-portal .
systemctl restart vpn-portal
systemctl status vpn-portal --no-pager
REMOTE

echo ""
echo "✓ vpn-portal updated and restarted on $1"
echo "  Logs: ssh $DROPLET journalctl -u vpn-portal -f"