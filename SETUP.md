# rzilient VPN Setup

Private access to the backoffice via WireGuard VPN + self-serve credential portal.

## Architecture

```
Team member → <your_vpn_domain> (Google SSO) → downloads WireGuard config
           → connects WireGuard → traffic exits via VPN droplet IP
           → <your_backoffice_domain> → Rack middleware checks IP → Trestle
```

**Droplet:** `<your_server_ip>` (`<your_server_hostname>`, DO ams3, $6/mo Ubuntu 24)
**VPN subnet:** `10.8.0.0/24` (server at `10.8.0.1`, peers from `10.8.0.2`)
**Portal:** `https://<your_vpn_domain>`
**Backoffice:** `https://<your_backoffice_domain>`

---

## 1. Deploy a VPN server

### Prerequisites

Before running the deploy script:

1. **Spin up a VPS** — Ubuntu 22 or 24, smallest tier ($6/mo DO, equivalent on other providers)
2. **Open firewall ports** on the provider dashboard:

| Protocol | Port | Source |
|---|---|---|
| UDP | 51820 | Anywhere |
| UDP | 443 | Anywhere |
| TCP | 22 | Your IP only |
| TCP | 80 | Anywhere |
| TCP | 443 | Anywhere |

> **Note:** UDP 443 is a fallback entry point for team members behind strict firewalls that block UDP 51820. The deploy script automatically sets up an iptables redirect from UDP 443 → 51820. TCP 443 remains used by nginx for HTTPS — they don't conflict since they use different protocols.

3. **Point DNS** to the server IP — e.g. `<your_vpn_domain> → <your_server_ip>`
4. **Add OAuth redirect URI** in Google Cloud Console:
   `https://<your_vpn_domain>/auth/callback`
5. **Update `vpn-portal.service`** in the repo with Google OAuth credentials:

```ini
Environment=GOOGLE_CLIENT_ID=<your_client_id>
Environment=GOOGLE_CLIENT_SECRET=<your_client_secret>
```

### Run the deploy script

The `deploy.sh` script handles everything remotely via SSH — WireGuard setup, portal build, nginx, SSL certificate.

```bash
# Basic deploy with defaults
./deploy.sh --host <your_server_ip>

# Full example with all options
./deploy.sh --host <your_server_ip> \
            --user root \
            --port 22 \
            --domain <your_vpn_domain> \
            --email <your_email> \
            --subnet 10.8.0 \
            --ssh-key ~/.ssh/id_rsa
```

### Script parameters

| Flag | Default | Description |
|---|---|---|
| `--host` | required | Server IP or hostname |
| `--user` | `root` | SSH user |
| `--port` | `22` | SSH port |
| `--domain` | `<your_vpn_domain>` | Portal domain |
| `--email` | `<your_email>` | Let's Encrypt email |
| `--subnet` | `10.8.0` | WireGuard subnet prefix |
| `--ssh-key` | `~/.ssh/id_rsa` | Path to SSH private key |

### What the script does

- Installs WireGuard, nginx, certbot, Go
- Generates WireGuard server keys
- Builds and deploys the vpn-portal Go app
- Configures nginx with SSL via Let's Encrypt
- Auto-injects server public key, IP, subnet and domain into the service file
- Starts and enables all services

### After deploy

Add the new server IP to the backoffice `VPN_ALLOWED_IPS` env var in the DO app spec:

```
VPN_ALLOWED_IPS=<your_server_ip>
```

---

## 2. Updating an existing deployment

Use `update.sh` (bash) or `update.fish` (fish) to push code or template changes:

```bash
./update.sh <your_server_ip>
# or
./update.fish <your_server_ip>
```

This copies `main.go`, `go.mod` and `templates/` to the server, rebuilds and restarts the portal.

---

## 3. Manual peer management (server-side)

Useful for testing or adding a peer without the portal.

```bash
# SSH into the server
ssh root@<your_server_ip>

# Generate key pair
mkdir -p /etc/wireguard/peers
wg genkey | tee /etc/wireguard/peers/alice_private.key | wg pubkey > /etc/wireguard/peers/alice_public.key

# Add peer (use next available IP: 10.8.0.2, 10.8.0.3, etc.)
wg set wg0 peer $(cat /etc/wireguard/peers/alice_public.key) allowed-ips 10.8.0.2/32
wg-quick save wg0

# Verify
wg show
```

Send the following config file to the user:

```ini
[Interface]
PrivateKey = <contents of alice_private.key>
Address = 10.8.0.2/32
DNS = 1.1.1.1

[Peer]
PublicKey = <your_server_public_key>
Endpoint = <your_server_ip>:51820
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25
```

To revoke a manually created peer:

```bash
wg set wg0 peer <their_public_key> remove
wg-quick save wg0
```

> **Note:** Manually created peers are not tracked in the portal's `state.json`.

---

## 4. Team member onboarding

1. Go to `https://<your_vpn_domain>`
2. Log in with `@rzilient.club` or `@rzilient.tech` Google account
3. Click **"Generate my credentials"**
4. Download the `.conf` file or scan the QR code

### Install on Linux (bash)

```bash
# On Arch: sudo pacman -S wireguard-tools openresolv
chmod +x install-vpn.sh
./install-vpn.sh ~/Downloads/rzilient-vpn.conf

# Verify
curl ifconfig.me  # should return <your_server_ip>

# Disconnect
sudo wg-quick down rzilient-vpn

# Auto-start on boot
sudo systemctl enable --now wg-quick@rzilient-vpn
```

### Install on Linux (fish)

```fish
chmod +x install-vpn.fish
./install-vpn.fish ~/Downloads/rzilient-vpn.conf
```

### Install on macOS (GUI — recommended)

1. Install **WireGuard** from the Mac App Store
2. Open WireGuard → **"+"** → **"Import tunnel(s) from file"**
3. Select `rzilient-vpn.conf`
4. Toggle on from the menu bar

### Install on macOS (Homebrew)

```bash
brew install wireguard-tools
sudo cp rzilient-vpn.conf /etc/wireguard/rzilient-vpn.conf
sudo wg-quick up rzilient-vpn
```

---

## 5. Revoking access

```bash
# SSH into the server
ssh root@<your_server_ip>

# Find their public key
cat /etc/wireguard/peers/state.json

# Remove from WireGuard
wg set wg0 peer <their_public_key> remove
wg-quick save wg0

# Remove from state.json
nano /etc/wireguard/peers/state.json
```

---

## 6. Backoffice Rails app

### `app/middleware/vpn_restriction.rb`

```ruby
class VpnRestriction
  def initialize(app)
    @app = app
  end

  def call(env)
    ip = env["HTTP_X_FORWARDED_FOR"]&.split(",")&.first&.strip || env["REMOTE_ADDR"]

    unless allowed?(ip)
      return [403, { "Content-Type" => "text/html" }, [forbidden_html]]
    end

    @app.call(env)
  end

  private

  def allowed?(ip)
    allowed_ips.include?(ip)
  end

  def allowed_ips
    ENV.fetch("VPN_ALLOWED_IPS", "").split(",").map(&:strip)
  end

  def forbidden_html
    <<~HTML
      <html>
        <body style="font-family: sans-serif; text-align: center; padding-top: 10vh">
          <h1>403 — Access Denied</h1>
          <p>Connect to the rzilient VPN to access this page.</p>
        </body>
      </html>
    HTML
  end
end
```

### `config/application.rb`

```ruby
require_relative '../app/middleware/vpn_restriction'

module Backoffice
  class Application < Rails::Application
    # ...
    config.middleware.insert_before 0, VpnRestriction if Rails.env.production?
  end
end
```

### DO App Platform env var

```
VPN_ALLOWED_IPS=<your_server_ip>
```

Add comma-separated IPs for multiple VPN servers:

```
VPN_ALLOWED_IPS=<your_server_ip>,<second_server_ip>
```

### Deployment order

1. Connect to WireGuard, verify `curl ifconfig.me` returns `<your_server_ip>`
2. Add `VPN_ALLOWED_IPS` to DO app spec and redeploy
3. Merge middleware changes to backoffice repo
4. Test `<your_backoffice_domain>` without VPN → `403`
5. Test `<your_backoffice_domain>` with VPN → login page ✓

---

## 7. DO App spec ingress order

Reordered so `/ext/*` services are reachable before the `/` catch-all:

```yaml
ingress:
  rules:
  - /inflow/orders     → inflow-orders
  - /inflow/companies  → inflow-companies
  - /api               → backend-production
  - /auth              → backend-production
  - /ext/authorization → authorization-service
  - /ext/devices       → getdevices-service
  - /ext/credentials   → credentials-service
  - /ext/metrics       → telemetry-service
  - /                  → backoffice
```

---

## 8. Maintenance

### Check WireGuard status
```bash
wg show
```

### Check portal logs
```bash
journalctl -u vpn-portal -f
```

### Restart portal
```bash
systemctl restart vpn-portal
```

### Stop portal (when no new credentials needed)
```bash
systemctl stop vpn-portal
systemctl disable vpn-portal  # optional
```

### Re-enable portal
```bash
systemctl enable --now vpn-portal
```

### Apply system updates
```bash
apt upgrade -y
reboot
```

After reboot all services (WireGuard, nginx, vpn-portal) restart automatically.

---

## 9. Deploying on a new provider (Scaleway, Hetzner, Vultr, etc.)

The `deploy.sh` script is provider-agnostic — any Ubuntu 22/24 VPS works.

```bash
# Example: Scaleway with a separate domain and subnet
./deploy.sh --host <scaleway_ip> \
            --domain vpn-fr.rzilient.tech \
            --subnet 10.9.0 \
            --email <your_email>
```

### Additional steps for a second server

**DNS** — use a different subdomain to run both simultaneously:
- `<your_vpn_domain>` → DO ams3 (`<your_server_ip>`)
- `vpn-fr.rzilient.tech` → Scaleway (`<new_ip>`)

**Google Cloud Console** — add the new redirect URI:
```
https://vpn-fr.rzilient.tech/auth/callback
```

**Backoffice** — add new server IP to `VPN_ALLOWED_IPS`:
```
VPN_ALLOWED_IPS=<your_server_ip>,<new_server_ip>
```

**Use a different subnet** per server to avoid conflicts:

| Server | Subnet | Server IP |
|---|---|---|
| DO ams3 (current) | `10.8.0.0/24` | `10.8.0.1` |
| Scaleway / new | `10.9.0.0/24` | `10.9.0.1` |

### When to add a second server

- Team members in a specific region complain about speed
- You want redundancy in case one server goes down
- You hit 20+ active peers on the current droplet