# vpn-portal — Technical Architecture

## Overview

`vpn-portal` is a self-serve WireGuard VPN credential portal with Google SSO. It is distributed as a Docker image and deployed to cloud servers (DigitalOcean or Scaleway) either manually via `deploy.sh` or automatically via the rzilient platform provisioning service.

---

## High level architecture

```
Developer
  └── push to main
        └── GitHub Actions
              ├── build Docker image (golang:1.25-alpine)
              ├── embed GIT_SHA via --build-arg
              └── push to registry.digitalocean.com/rzilient-do-containers/vpn-portal:latest

rzilient Platform (Rails backend)
  └── POST /api/v1/vpn/instances
        └── VpnProvisionJob (Sidekiq · vpn queue)
              └── Vpn::Provisioner
                    ├── DO API → create droplet (ams3) OR Scaleway API → create instance (fr-par-1)
                    ├── DO DNS API → create A record (rzilient.tech)
                    ├── Wait for SSH
                    └── Run setup_commands via net-ssh
                          ├── Install WireGuard, nginx, certbot, Docker, doctl
                          ├── doctl registry login → docker pull vpn-portal:latest
                          ├── Create /etc/vpn-portal.env
                          ├── docker run (with Docker socket + vpn-update mounts)
                          ├── nginx + certbot SSL
                          └── Install /usr/local/bin/vpn-update

VPN Server (Ubuntu · Docker)
  ├── vpn-portal container (Go app)
  │     ├── Google OAuth2 login
  │     ├── WireGuard peer generation
  │     ├── Admin panel (peer management, live stats, update)
  │     └── /health endpoint
  ├── WireGuard (wg0) — UDP 51820 + 443 fallback
  ├── nginx — reverse proxy + SSL (Let's Encrypt)
  └── /usr/local/bin/vpn-update — self-update script (host)

Client
  └── Google SSO login → generate WireGuard config → connect via UDP
```

---

## Components

### vpn-portal (Go application)

The core application. A single Go binary serving HTTP on port 8080.

**Endpoints:**

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/` | Session | Home — shows config or generate button |
| `POST` | `/generate` | Session | Generate WireGuard keypair + assign IP |
| `GET` | `/download` | Session | Download `.conf` file |
| `GET` | `/qr` | Session | QR code PNG for mobile import |
| `GET` | `/auth/login` | — | Start Google OAuth2 flow |
| `GET` | `/auth/callback` | — | OAuth2 callback |
| `GET` | `/auth/logout` | — | Clear session |
| `GET` | `/health` | — | Health check — returns `{"status":"ok"}` |
| `GET` | `/manifest.json` | — | PWA manifest |
| `GET` | `/static/*` | — | CSS, JS assets |
| `GET` | `/admin` | Admin token | Admin panel |
| `POST` | `/admin/block` | Admin token | Block a peer |
| `POST` | `/admin/unblock` | Admin token | Unblock a peer |
| `POST` | `/admin/revoke` | Admin token | Permanently remove a peer |
| `POST` | `/admin/add-peer` | Admin token | Add peer manually |
| `GET` | `/admin/config` | Admin token | Get peer WireGuard config |
| `GET` | `/admin/stats` | Admin token | Live RX/TX stats from `wg show dump` |
| `GET` | `/admin/version` | Admin token | Compare running vs registry image digest |
| `POST` | `/admin/update` | Admin token | Trigger `/usr/local/bin/vpn-update` |

**State:**

All peer state is stored in `/etc/wireguard/peers/state.json` (JSON, 600 permissions). No database. State persists across container restarts via the `/etc/wireguard` volume mount.

```json
{
  "peers": [
    {
      "email": "user@company.com",
      "name": "Full Name",
      "public_key": "...",
      "private_key": "...",
      "assigned_ip": "10.8.0.2",
      "created_at": "2026-05-01T12:00:00Z",
      "blocked": false
    }
  ]
}
```

**Session:**

Cookie-based. Email + name base64 encoded in `vpn_session` cookie (HttpOnly, Secure, 7 days).

**WireGuard peer lifecycle:**

1. User logs in via Google OAuth — email domain checked against `ALLOWED_DOMAINS`
2. `nextIP()` finds the next available IP in the subnet (`.2` → `.253`)
3. `wg genkey | wg pubkey` generates keypair
4. `wg set wg0 peer <pubkey> allowed-ips <ip>/32` adds peer to WireGuard
5. `wg-quick save wg0` persists to `wg0.conf`
6. State saved to `state.json`

**Self-update:**

The container mounts `/var/run/docker.sock` and `/usr/local/bin/vpn-update` from the host. When admin clicks "update now":

1. `POST /admin/update` — runs `vpn-update` in a goroutine (non-blocking)
2. `vpn-update` runs on the host via the Docker socket mount:
   - `doctl registry login`
   - `docker pull vpn-portal:latest`
   - `docker stop vpn-portal && docker rm vpn-portal`
   - `docker run -d ... vpn-portal:latest` (same flags, including socket mounts)
3. Container restarts with new image (~30 seconds downtime)
4. Admin panel re-checks version after 60 seconds

**Version detection:**

- `GIT_SHA` is baked into the binary at build time via `-ldflags "-X main.buildSHA=${GIT_SHA}"`
- `GET /admin/version` compares running container's image digest against the registry's latest digest using `docker inspect` and `docker manifest inspect`
- Returns `{"update_available": true/false, "current_sha": "abc1234"}`

---

### Docker image

**Registry:** `registry.digitalocean.com/rzilient-do-containers/vpn-portal:latest`

**Build:** Multi-stage — `golang:1.25-alpine` builder, `alpine:3.19` runtime.

**Runtime packages:** `wireguard-tools`, `ca-certificates`, `tzdata`, `docker-cli`

**Build args:**

| Arg | Description |
|-----|-------------|
| `GIT_SHA` | Full git commit SHA — baked into binary via ldflags |

**CI/CD:** GitHub Actions (`docker.yml`) — triggers on push to `main`. Uses 1Password service account to fetch `DO_API_TOKEN`.

---

### Environment variables

All configuration is passed via `/etc/vpn-portal.env` (Docker `--env-file`).

| Variable | Description | Auto-injected |
|----------|-------------|---------------|
| `WG_SERVER_PUBLIC_KEY` | Server WireGuard public key | ✅ deploy script |
| `WG_SERVER_ENDPOINT` | Server public IPv4 | ✅ deploy script |
| `BASE_URL` | Portal public URL | ✅ deploy script |
| `VPN_SUBNET` | WireGuard subnet prefix (e.g. `10.9.0`) | ✅ deploy script |
| `WG_INTERFACE` | WireGuard interface name (default: `wg0`) | ✅ deploy script |
| `STATE_FILE` | Path to state JSON | ✅ deploy script |
| `PORT` | HTTP port (default: `8080`) | ✅ deploy script |
| `GOOGLE_CLIENT_ID` | Google OAuth client ID | 🔑 manual / `init` |
| `GOOGLE_CLIENT_SECRET` | Google OAuth client secret | 🔑 manual / `init` |
| `ALLOWED_DOMAINS` | Comma-separated email domains | 🔑 manual / `init` |
| `ADMIN_TOKEN` | Admin panel access token | 🔑 manual / `init` |
| `GIT_SHA` | Build SHA (baked at build time) | ✅ build arg |
| `REGISTRY` | DO registry URL | ✅ deploy script |
| `DEV_MODE` | Skip `wg` commands for local dev | local only |

---

### Docker run flags

```bash
docker run -d \
  --name vpn-portal \
  --restart unless-stopped \
  --network host \
  --cap-add NET_ADMIN \
  --env-file /etc/vpn-portal.env \
  -v /etc/wireguard:/etc/wireguard \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /usr/local/bin/vpn-update:/usr/local/bin/vpn-update \
  registry.digitalocean.com/rzilient-do-containers/vpn-portal:latest
```

| Flag | Reason |
|------|--------|
| `--network host` | WireGuard requires host network access |
| `--cap-add NET_ADMIN` | Required to run `wg` and `ip` commands |
| `-v /etc/wireguard` | Peer state and WG keys persist across container updates |
| `-v /var/run/docker.sock` | Self-update: container talks to host Docker daemon |
| `-v /usr/local/bin/vpn-update` | Self-update script lives on host, mounted read-only |

---

### Networking

| Protocol | Port | Purpose |
|----------|------|---------|
| TCP | 22 | SSH (your IP only) |
| TCP | 80 | nginx HTTP (certbot challenge) |
| TCP | 443 | nginx HTTPS (portal) |
| UDP | 51820 | WireGuard |
| UDP | 443 | WireGuard fallback (iptables redirect → 51820) |
| TCP | 8080 | vpn-portal (internal only, not exposed) |

**iptables rule (UDP 443 fallback):**
```bash
iptables -t nat -A PREROUTING -i <NET_IF> -p udp --dport 443 -j REDIRECT --to-port 51820
```

---

### WireGuard configuration

**Server (`/etc/wireguard/wg0.conf`):**

```ini
[Interface]
PrivateKey = <server_private_key>
Address = 10.x.0.1/24
ListenPort = 51820
PostUp = iptables -A FORWARD -i wg0 -j ACCEPT; iptables -t nat -A POSTROUTING -o <NET_IF> -j MASQUERADE
PostDown = iptables -D FORWARD -i wg0 -j ACCEPT; iptables -t nat -D POSTROUTING -o <NET_IF> -j MASQUERADE
```

Network interface (`<NET_IF>`) is auto-detected at deploy time via:
```bash
ip route show default | awk '/default/ {print $5}' | head -1
```

**Client config template:**

```ini
[Interface]
PrivateKey = <client_private_key>
Address = 10.x.0.y/32
DNS = 1.1.1.1

[Peer]
PublicKey = <server_public_key>
Endpoint = <server_ip>:51820
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25
```

---

## Infrastructure

### Active instances

| Instance | Provider | Region | Subnet | Domain |
|----------|----------|--------|--------|--------|
| DO ams3 | DigitalOcean | Amsterdam | `10.8.0.0/24` | `vpn.rzilient.tech` |
| SCW par1 | Scaleway | Paris | `10.9.0.0/24` | `vpn-fr.rzilient.tech` |

**Subnet assignment:** Sequential — `10.8.0`, `10.9.0`, `10.10.0`, etc. Each instance owns a `/24` subnet. Client IPs are assigned `.2` through `.253`.

**DNS:** All DNS records managed via DigitalOcean DNS API (rzilient.tech domain). Even Scaleway instances use DO DNS.

---

## Rails provisioning service

The rzilient Rails backend can provision new VPN instances automatically via API.

### Model

```ruby
VpnInstance
  company_id:       uuid     # acts_as_tenant
  provider:         string   # digitalocean | scaleway
  region:           string   # ams3 | fr-par-1
  server_ip:        string
  domain:           string   # unique
  subnet:           string   # e.g. 10.9.0
  status:           string   # pending | provisioning | configuring | active | failed | destroyed
  droplet_id:       string   # provider instance ID
  wg_public_key:    string
  error_message:    text
  allowed_domains:  string
  secret:           string   # ADMIN_TOKEN
  oidc_issuer:      string   # optional — for non-Google SSO
  oidc_client_id:   string
  oidc_client_secret: string
```

### API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/vpn/instances` | List company instances |
| `GET` | `/api/v1/vpn/instances/:id` | Get instance (poll for status) |
| `POST` | `/api/v1/vpn/instances` | Create + provision new instance |
| `DELETE` | `/api/v1/vpn/instances/:id` | Destroy instance |
| `POST` | `/api/v1/vpn/instances/:id/update` | Trigger image update on single instance |
| `POST` | `/api/v1/vpn/instances/update_all` | Trigger image update on all active instances |

**Create request:**

```json
{
  "region": "ams3",
  "allowed_domains": "client.com",
  "oidc_issuer": "https://...",
  "oidc_client_id": "...",
  "oidc_client_secret": "..."
}
```

**Status flow:**

```
pending → provisioning → configuring → active
                                     ↘ failed
active → destroyed
```

### Provisioner flow

1. Create server via provider API (DO or Scaleway)
2. Poll for public IPv4
3. Create DNS A record via DO API
4. Poll for SSH availability
5. Run `setup_commands` via `net-ssh`:
   - Install dependencies (nginx, certbot, wireguard-tools, docker.io, doctl)
   - Generate WireGuard keys + config
   - Set up iptables rules
   - Authenticate with DO Container Registry via `doctl`
   - Pull `vpn-portal:latest`
   - Create `/etc/vpn-portal.env`
   - Install `/usr/local/bin/vpn-update`
   - Start Docker container
   - Configure nginx + obtain SSL certificate
6. Fetch WireGuard public key from server
7. Mark instance `active`

### Required environment variables (Rails)

```
DO_API_TOKEN              — DigitalOcean API token (droplets + DNS + registry)
SCW_SECRET_KEY            — Scaleway secret key
SCW_PROJECT_ID            — Scaleway project ID
SCW_ORGANIZATION_ID       — Scaleway organization ID
VPN_GOOGLE_CLIENT_ID      — Shared Google OAuth client ID
VPN_GOOGLE_CLIENT_SECRET  — Shared Google OAuth client secret
VPN_SSH_KEY_BASE64        — Base64-encoded SSH private key (no file path needed)
```

**SSH key:** Stored as base64 in env var, decoded at runtime via `net-ssh` `key_data:` option. No file needed on DO App Platform.

---

## Deployment

### Manual deploy (new server)

```bash
# 1. Configure secrets locally
nano vpn-portal.service  # set GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, ALLOWED_DOMAINS, ADMIN_TOKEN

# 2. Deploy
export DO_API_TOKEN=$(op read "op://Engineering/API_Production/DO_API_TOKEN")
./deploy.sh deploy --host <ip> --domain <domain> --email <email> --subnet <subnet>

# 3. Push secrets
./deploy.sh init --host <ip>
```

### Update existing server

```bash
# Via deploy script (SSH)
./deploy.sh update --host <ip>

# Via admin panel (no SSH)
# → Open https://<domain>/admin?token=<ADMIN_TOKEN>
# → Click "update now" when badge shows "↑ update available"
```

### Local development

```bash
cp .env.example .env
# Edit .env — set DEV_MODE=true to skip wg commands

# Run with Go directly
go run main.go

# Or with Docker
./docker-run.fish run
```

### Build and push image

```bash
# Push triggers GitHub Actions automatically on merge to main
git push origin main

# Or build + push manually
./docker-run.fish push
```

---

## Admin panel

Access at `https://<domain>/admin?token=<ADMIN_TOKEN>`

| Feature | Description |
|---------|-------------|
| Peer table | All peers with status, VPN IP, created date |
| Live RX/TX | Traffic stats polled every 30s from `wg show dump` |
| Online indicator | Green dot if last handshake < 3 minutes ago |
| Search | Filter by email, name, or VPN IP |
| Sort | Click any column header |
| Block / Unblock | Remove/re-add peer from WireGuard |
| Revoke | Permanently delete peer and credentials |
| Add peer | Manually provision credentials without Google SSO |
| Copy config | Copy WireGuard `.conf` to clipboard |
| Export CSV | Download peer list with traffic stats |
| Version badge | Shows current image SHA — "↑ update available" when registry has newer image |
| Update button | One-click update — pulls latest image and restarts container |

---

## Security notes

- `vpn-portal.service` and `/etc/vpn-portal.env` contain secrets — never commit real credentials
- `/etc/vpn-portal.env` has `600` permissions on the server
- `state.json` contains WireGuard private keys — never commit it
- `*.conf` files contain WireGuard private keys — never commit them
- Docker socket mount (`/var/run/docker.sock`) gives the container full Docker access — only used for self-update
- The portal must be publicly accessible (Google OAuth requires it)
- Backoffice access is restricted by VPN server IP via Rails middleware (`VPN_ALLOWED_IPS`)
- Admin panel is protected by `ADMIN_TOKEN` query parameter — use a strong random token (`SecureRandom.hex(24)`)

---

## Project structure

```
vpn-portal/
  main.go                        # Go app — all handlers
  go.mod / go.sum                # Go dependencies
  manifest.json                  # PWA manifest
  Dockerfile                     # Multi-stage build (golang:1.25 + alpine:3.19)
  .dockerignore
  vpn-portal.service             # Secrets reference file (not committed with real values)
  nginx.conf                     # HTTP-only — certbot adds SSL block
  deploy.sh                      # deploy / update / init commands
  docker-run.fish                # Local build + push + run (fish shell)
  test-deploy.sh                 # Reset server + full redeploy (testing)
  install-vpn.sh                 # Client WireGuard config installer (bash)
  install-vpn.fish               # Client WireGuard config installer (fish)
  .env.example                   # Local dev env template
  templates/
    home.html                    # Portal UI
    admin.html                   # Admin panel
    unauthorized.html            # Access denied page
  static/
    css/
      home.css
      admin.css
      unauthorized.css
    js/
      admin.js                   # Stats polling, sort, search, export, update
  .github/
    workflows/
      docker.yml                 # GitHub Actions — build + push on main
  ARCHITECTURE.md                # This document
  README.md                      # Setup and usage guide
  NEW_INSTANCE.md                # Step by step new instance deployment guide
```
