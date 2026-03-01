# Installation Guide

This guide covers installing Redoubt on a production server. Redoubt is a self-hosted team communication platform with text, voice, and video channels.

## Requirements

- **OS:** Ubuntu 22.04+ or Debian 12+ (amd64 or arm64)
- **CPU:** 2+ vCPUs
- **RAM:** 4+ GB
- **Network:** Public IP address with ports 80 and 443 available
- **DNS:** A domain name with an A record pointing to the server's IP address

## Quick Install

Run the installer script on your server:

```bash
curl -fsSL https://raw.githubusercontent.com/redoubtapp/redoubt-api/main/install.sh | bash
```

The installer is interactive and will prompt for your domain name and email address.

## What the Installer Does

The installer performs the following steps in order:

1. **System checks** -- Verifies the operating system, architecture, available memory, and that the script is running as root.
2. **Docker installation** -- Installs Docker Engine and the Docker Compose plugin if not already present.
3. **Domain and email prompts** -- Asks for the domain name and an email address for Let's Encrypt TLS certificates.
4. **DNS verification** -- Resolves the domain and confirms the A record points to the server's public IP.
5. **Secret generation** -- Generates cryptographically secure values for `POSTGRES_PASSWORD`, `JWT_SECRET`, `STORAGE_MASTER_KEY`, `LIVEKIT_API_KEY`, `LIVEKIT_API_SECRET`, and `ADMIN_SESSION_SECRET`.
6. **File download** -- Creates `/opt/redoubt` and downloads `docker-compose.prod.yml`, `Caddyfile`, `config/config.yaml`, and `config/livekit.yaml` from the repository.
7. **Environment file creation** -- Writes all secrets and configuration values to `/opt/redoubt/.env`.
8. **Firewall configuration** -- Configures `ufw` to allow ports 22, 80, 443, and the LiveKit UDP range (60000-60100).
9. **Service startup** -- Pulls container images and starts all services with `docker compose up -d`.
10. **Health check** -- Waits for the API to become healthy by polling the `/health` endpoint.
11. **Bootstrap code** -- Prints a one-time invite code used to register the first user (instance administrator).

## Manual Installation

If you prefer to install without the automated script, follow these steps.

### 1. Install Docker

Install Docker Engine and the Compose plugin by following the official guide:

- [Install Docker Engine on Ubuntu](https://docs.docker.com/engine/install/ubuntu/)
- [Install Docker Engine on Debian](https://docs.docker.com/engine/install/debian/)

Verify the installation:

```bash
docker --version
docker compose version
```

### 2. Create the Install Directory

```bash
mkdir -p /opt/redoubt/config
cd /opt/redoubt
```

### 3. Download Configuration Files

```bash
REPO_URL="https://raw.githubusercontent.com/redoubtapp/redoubt-api/main"

curl -fsSL "$REPO_URL/docker-compose.prod.yml" -o docker-compose.yml
curl -fsSL "$REPO_URL/Caddyfile" -o Caddyfile
curl -fsSL "$REPO_URL/config/config.yaml" -o config/config.yaml
curl -fsSL "$REPO_URL/config/livekit.yaml" -o config/livekit.yaml
```

### 4. Generate Secrets

Generate all required secrets:

```bash
POSTGRES_PASSWORD=$(openssl rand -hex 32)
JWT_SECRET=$(openssl rand -base64 64 | tr -d '\n')
STORAGE_MASTER_KEY=$(openssl rand -base64 32 | tr -d '\n')
LIVEKIT_API_KEY="API$(openssl rand -hex 12)"
LIVEKIT_API_SECRET=$(openssl rand -base64 32 | tr -d '\n')
ADMIN_SESSION_SECRET=$(openssl rand -base64 32 | tr -d '\n')
```

### 5. Create the .env File

Replace `your.domain.com` and `you@example.com` with your actual values:

```bash
cat > /opt/redoubt/.env << EOF
DOMAIN=your.domain.com
ACME_EMAIL=you@example.com
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
JWT_SECRET=${JWT_SECRET}
STORAGE_MASTER_KEY=${STORAGE_MASTER_KEY}
LIVEKIT_API_KEY=${LIVEKIT_API_KEY}
LIVEKIT_API_SECRET=${LIVEKIT_API_SECRET}
ADMIN_SESSION_SECRET=${ADMIN_SESSION_SECRET}
EOF
```

Set restrictive permissions on the file:

```bash
chmod 600 /opt/redoubt/.env
```

### 6. Configure the Firewall

```bash
ufw allow 22/tcp      # SSH
ufw allow 80/tcp      # HTTP (Let's Encrypt + redirect)
ufw allow 443/tcp     # HTTPS
ufw allow 60000:60100/udp  # LiveKit WebRTC media
ufw --force enable
```

### 7. Start Services

```bash
cd /opt/redoubt
docker compose pull
docker compose up -d
```

### 8. Verify the Deployment

Watch the logs until all services are healthy:

```bash
docker compose logs -f
```

Test the health endpoint:

```bash
curl -k https://localhost/health
```

## First User Setup

After installation, you need to register the first user who will become the instance administrator.

1. **Get the bootstrap invite code.** The installer prints it at the end of the installation. If you missed it, retrieve it from the logs:

   ```bash
   docker compose logs redoubt-api | grep "bootstrap invite"
   ```

2. **Register via a Redoubt client.** Open the Redoubt desktop or web client, point it at your domain, and register a new account using the bootstrap invite code.

3. **First user becomes admin.** The first registered user is automatically granted instance administrator privileges.

## Admin Panel Access

The admin panel listens on port 9091 and is only accessible via localhost. Use an SSH tunnel to access it securely.

1. **Open an SSH tunnel:**

   ```bash
   ssh -L 9091:localhost:9091 user@your-server
   ```

2. **Open the admin panel** in your browser at [http://localhost:9091](http://localhost:9091).

3. **Log in** with your instance administrator credentials.

4. **Available features:**
   - Dashboard with instance overview
   - User management (invite, suspend, delete)
   - Space management
   - Audit logs

## Post-Install Configuration

All optional configuration is done by adding environment variables to `/opt/redoubt/.env` and restarting services.

### Email (Transactional)

Email enables account verification and password reset flows.

```bash
# Add to .env
RESEND_API_KEY=re_your_api_key_here
```

```bash
cd /opt/redoubt
docker compose up -d
```

### S3 Storage (File Uploads)

Configure an S3-compatible storage provider for file uploads and media.

```bash
# Add to .env
S3_ACCESS_KEY=your_access_key
S3_SECRET_KEY=your_secret_key
```

```bash
cd /opt/redoubt
docker compose up -d
```

### Monitoring (Prometheus + Grafana)

Enable the optional monitoring stack:

```bash
cd /opt/redoubt
docker compose --profile monitoring up -d
```

Grafana is available at `http://your-server:3000`. The default password is `admin` unless `GRAFANA_PASSWORD` is set in `.env`.

## Uninstalling

To completely remove Redoubt and all its data:

```bash
cd /opt/redoubt
docker compose down -v
rm -rf /opt/redoubt
```

> **Warning:** This permanently deletes all data including messages, files, and user accounts.
