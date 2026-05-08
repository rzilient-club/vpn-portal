# vpn-portal

Self-serve WireGuard VPN credential portal with Google SSO. Team members log in with their org Google account and get a personal WireGuard config generated automatically.

## Features

- Google OAuth restricted to specific email domains
- Auto-generates WireGuard key pairs per user
- QR code for mobile setup (iOS / Android)
- Config file download
- Single device per account
- Admin panel — view peers, block, unblock, revoke, live traffic stats, export CSV
- State stored in a simple JSON file — no database needed
- PWA ready — installable on mobile

## Stack

- **Go** — HTTP server, OAuth2, WireGuard peer management
- **WireGuard** — VPN tunnel
- **nginx** — reverse proxy + HTTPS
- **Let's Encrypt** — SSL via certbot
- **Docker** — containerised deployment
- **DO Container Registry** — image hosting

## Docker image

The portal is distributed as a Docker image via DO Container Registry:

```
registry.digitalocean.com/rzilient/vpn-portal:latest
```

A new image is built and pushed automatically on every push to `main` via GitHub Actions.

## Prerequisites

- Ubuntu 22, 24 or 25 VPS (DigitalOcean, Scaleway, Hetzner, etc.)
- Docker installed on the server
- A domain pointing to the server IP
- Google OAuth credentials from Google Cloud Console
- Authorized redirect URI: `https://<your_vpn_domain>/auth/callback`
- `DO_API_TOKEN` with read access to the container registry

## Configuration

Before deploying, update `vpn-portal.service` with your credentials:

```ini
# ── Set manually via: ./deploy.sh init --host x.x.x.x ────────────────────────
Environment=GOOGLE_CLIENT_ID=<your_google_client_id>
Environment=GOOGLE_CLIENT_SECRET=<your_google_client_secret>
Environment=ALLOWED_DOMAINS=yourdomain.com,yourdomain.io
Environment=ADMIN_TOKEN=<your_secure_admin_token>
```

`WG_SERVER_PUBLIC_KEY`, `WG_SERVER_ENDPOINT`, `BASE_URL` and `VPN_SUBNET` are auto-injected by the deploy script.

## Commands

### Fresh deployment
```bash
./deploy.sh deploy --host <server_ip> \
                   --domain <your_vpn_domain> \
                   --email <your_email> \
                   --subnet 10.8.0
```

### Push secrets after deploy
```bash
./deploy.sh init --host <server_ip>
```

### Update to latest Docker image
```bash
./deploy.sh update --host <server_ip>
```

### All deploy options

| Flag | Required | Default | Description |
|---|---|---|---|
| `--host` | ✅ | — | Server IP or hostname |
| `--domain` | ✅ | — | VPN portal domain |
| `--email` | ✅ | — | Let's Encrypt email |
| `--user` | — | `root` | SSH user |
| `--port` | — | `22` | SSH port |
| `--subnet` | — | `10.8.0` | WireGuard subnet prefix |

## Firewall ports

| Protocol | Port | Source |
|---|---|---|
| TCP | 22 | Your IP only |
| TCP | 80 | Anywhere |
| TCP | 443 | Anywhere |
| UDP | 443 | Anywhere (fallback for strict firewalls) |
| UDP | 51820 | Anywhere |

## Admin panel

Access at `https://<your_vpn_domain>/admin?token=<ADMIN_TOKEN>`

- View all peers with live RX/TX traffic stats
- Online/offline status per peer
- Block, unblock or permanently revoke access
- Copy WireGuard config to clipboard
- Export peers as CSV
- Search and filter by email, name or IP
- Sort by any column

## Client installation

### macOS
Install **WireGuard** from the Mac App Store, import the `.conf` file.

### Linux (bash)
```bash
chmod +x install-vpn.sh
./install-vpn.sh ~/Downloads/rzilient-vpn.conf
```

### Linux (fish)
```fish
chmod +x install-vpn.fish
./install-vpn.fish ~/Downloads/rzilient-vpn.conf
```

## Development

### Run locally
```bash
cp .env.example .env
# Edit .env with your credentials
go run main.go
```

### Build Docker image locally
```bash
docker build -t vpn-portal .
docker run --rm -p 8080:8080 --env-file .env vpn-portal
```

### CI/CD
On every push to `main`, GitHub Actions:
1. Builds the Docker image
2. Tags it as `latest` and with the short commit SHA
3. Pushes to `registry.digitalocean.com/rzilient/vpn-portal`

## Project structure

```
vpn-portal/
  main.go                     # Go app — OAuth, WireGuard, admin, stats
  go.mod                      # Go module
  go.sum                      # Go checksums
  manifest.json               # PWA manifest
  Dockerfile                  # Multi-stage Docker build
  .dockerignore               # Docker build exclusions
  vpn-portal.service          # systemd/secrets reference file
  nginx.conf                  # nginx reverse proxy (HTTP — certbot adds SSL)
  deploy.sh                   # deploy / update / init commands
  install-vpn.sh              # client config installer (bash)
  install-vpn.fish            # client config installer (fish)
  templates/
    home.html                 # portal UI
    admin.html                # admin panel
    unauthorized.html         # access denied page
  static/
    css/
      home.css
      admin.css
      unauthorized.css
    js/
      admin.js
  .github/
    workflows/
      docker.yml              # GitHub Actions — build and push image
```

## Security notes

- `vpn-portal.service` contains secrets — never commit real credentials
- `/etc/vpn-portal.env` on the server contains secrets — created at deploy time with `600` permissions
- `state.json` contains WireGuard private keys — never commit it
- `*.conf` files contain WireGuard private keys — never commit them
- The portal must be publicly accessible (Google OAuth requires it)
- Backoffice access is restricted by VPN server IP via Rails middleware (`VPN_ALLOWED_IPS`)

## Deploying a new instance

See [NEW_INSTANCE.md](NEW_INSTANCE.md) for the full step by step guide.