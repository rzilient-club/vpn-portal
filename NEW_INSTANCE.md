# Deploying a New VPN Instance

Step by step guide to deploy and initialize a new rzilient VPN server.

---

## Prerequisites

- `DO_API_TOKEN` with read access to `registry.digitalocean.com/rzilient`
- DNS managed on DigitalOcean (`rzilient.tech`)
- SSH key added to the provider (named `rzilient-vpn`)
- `vpn-portal.service` with secrets filled in locally

---

## 1. Create the server

Spin up a **Ubuntu 22, 24 or 25** VPS:

- Minimum spec: 1 vCPU / 1GB RAM
- Recommended: 1 vCPU / 2GB RAM (e.g. Scaleway PLAY2-PICO)

Open firewall ports on the provider dashboard:

| Protocol | Port | Source |
|---|---|---|
| TCP | 22 | Your IP only (`x.x.x.x/32`) |
| TCP | 80 | Anywhere |
| TCP | 443 | Anywhere |
| UDP | 443 | Anywhere |
| UDP | 51820 | Anywhere |

---

## 2. Point DNS

Add an A record pointing your subdomain to the server IP:

```
vpn-xx.rzilient.tech → <server_ip>
```

Verify propagation before deploying:

```bash
dig vpn-xx.rzilient.tech +short
```

---

## 3. Add OAuth redirect URI

In **Google Cloud Console → APIs & Services → Credentials → your OAuth client** add:

```
https://vpn-xx.rzilient.tech/auth/callback
```

---

## 4. Configure `vpn-portal.service` locally

Fill in the 4 secrets:

```ini
Environment=GOOGLE_CLIENT_ID=<your_google_client_id>
Environment=GOOGLE_CLIENT_SECRET=<your_google_client_secret>
Environment=ALLOWED_DOMAINS=yourdomain.com,yourdomain.io
Environment=ADMIN_TOKEN=<your_secure_admin_token>
```

> `WG_SERVER_PUBLIC_KEY`, `WG_SERVER_ENDPOINT`, `BASE_URL` and `VPN_SUBNET`
> are injected automatically by the deploy script — do not set them manually.

---

## 5. Deploy

```bash
./deploy.sh deploy --host <server_ip> \
                   --domain vpn-xx.rzilient.tech \
                   --email eduard@rzilient.club \
                   --subnet 10.x.0
```

The script will:
- Install Docker, WireGuard, nginx, certbot
- Generate WireGuard keys
- Authenticate with DO Container Registry using `DO_API_TOKEN`
- Pull `registry.digitalocean.com/rzilient/vpn-portal:latest`
- Create `/etc/vpn-portal.env` with auto-injected values
- Start the container with WireGuard volume mount
- Obtain SSL certificate via Let's Encrypt

---

## 6. Push secrets

```bash
./deploy.sh init --host <server_ip>
```

Reads secrets from your local `vpn-portal.service` and injects them
into `/etc/vpn-portal.env` on the server, then restarts the container.

---

## 7. Add server IP to backoffice

In the DO app spec add the new server IP to `VPN_ALLOWED_IPS`:

```yaml
- key: VPN_ALLOWED_IPS
  value: 146.190.20.101,51.158.75.75,<new_server_ip>
```

---

## 8. Verify

```bash
# Portal accessible
open https://vpn-xx.rzilient.tech

# Container running
ssh root@<server_ip> docker ps

# WireGuard running
ssh root@<server_ip> wg show

# Logs
ssh root@<server_ip> docker logs -f vpn-portal
```

---

## Subnet reference

| Server | Region | Subnet |
|---|---|---|
| DO ams3 | Amsterdam | `10.8.0` |
| Scaleway par1 | Paris | `10.9.0` |
| Next server | — | `10.10.0` |

---

## Day-to-day commands

```bash
# Deploy fresh server
./deploy.sh deploy --host <ip> --domain <domain> --email <email> --subnet <subnet>

# Update to latest Docker image
./deploy.sh update --host <ip>

# Push updated secrets
./deploy.sh init --host <ip>
```

---

## Updating all existing instances

When a new Docker image is pushed, update all active instances:

```bash
# One at a time
./deploy.sh update --host 146.190.20.101
./deploy.sh update --host 51.158.75.75

# Or via Rails console
VpnInstance.active.each { |i| VpnUpdateJob.perform_later(i.id) }
```
