# vpn-portal

Self-serve WireGuard VPN credential portal with Google SSO. Team members log in with their org Google account and get a personal WireGuard config generated automatically.

## Features

- Google OAuth restricted to specific domains
- Auto-generates WireGuard key pairs per user
- QR code for mobile setup
- Config file download
- Single device per account
- State stored in a simple JSON file — no database needed

## Stack

- **Go** — HTTP server, OAuth2, WireGuard peer management
- **WireGuard** — VPN
- **nginx** — reverse proxy
- **Let's Encrypt** — SSL via certbot

## Prerequisites

- Ubuntu 22 or 24 VPS
- A domain pointing to the server IP
- Google OAuth credentials from Google Cloud Console
- Authorized redirect URI: `https://<your_vpn_domain>/auth/callback`

## Configuration

Before deploying, update `vpn-portal.service` with your credentials:

```ini
Environment=GOOGLE_CLIENT_ID=<your_google_client_id>
Environment=GOOGLE_CLIENT_SECRET=<your_google_client_secret>
Environment=ALLOWED_DOMAINS=yourdomain.com,yourdomain.io
Environment=BASE_URL=https://<your_vpn_domain>
```

`WG_SERVER_PUBLIC_KEY`, `WG_SERVER_ENDPOINT`, `VPN_SUBNET` and `BASE_URL` are auto-injected by the deploy script.

## Deploy

```bash
./deploy.sh --host <server_ip> \
            --domain <your_vpn_domain> \
            --email <your_email>
```

### All options

| Flag | Required | Default | Description |
|---|---|---|---|
| `--host` | ✅ | — | Server IP or hostname |
| `--domain` | ✅ | — | VPN portal domain |
| `--email` | ✅ | — | Let's Encrypt email |
| `--user` | — | `root` | SSH user |
| `--port` | — | `22` | SSH port |
| `--subnet` | — | `10.8.0` | WireGuard subnet prefix |
| `--ssh-key` | — | `~/.ssh/id_rsa` | SSH private key path |

## Update existing deployment

```bash
./update.sh <server_ip>
# or
./update.fish <server_ip>
```

## Firewall ports required on the server

| Protocol | Port | Source |
|---|---|---|
| UDP | 51820 | Anywhere |
| UDP | 443 | Anywhere (fallback for strict firewalls) |
| TCP | 22 | Your IP only |
| TCP | 80 | Anywhere |
| TCP | 443 | Anywhere |

## Project structure

```
vpn-portal/
  main.go                 # Go app
  go.mod                  # Go module
  vpn-portal.service      # systemd service — configure before deploying
  nginx.conf              # nginx reverse proxy
  templates/
    home.html             # Portal UI
    unauthorized.html     # Access denied page
  deploy.sh               # Fresh server deployment (bash)
  update.sh               # Update existing deployment (bash)
  update.fish             # Update existing deployment (fish)
  install-vpn.sh          # Client install helper (bash)
  install-vpn.fish        # Client install helper (fish)
```

## Security notes

- `vpn-portal.service` contains secrets — never commit real credentials
- `state.json` contains WireGuard private keys — never commit it
- `.conf` files contain private keys — never commit them
- The portal itself must be publicly accessible (Google OAuth requires it)
- Access to the protected resource (your backoffice) is restricted by server IP via middleware