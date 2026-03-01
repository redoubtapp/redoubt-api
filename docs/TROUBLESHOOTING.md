# Troubleshooting

This guide covers common issues encountered when installing and running Redoubt, along with their causes and fixes.

## Installation Issues

### DNS Not Resolving

**Symptom:** The installer fails with a DNS verification error, or Caddy cannot obtain a TLS certificate.

**Cause:** The domain's A record does not point to the server's public IP address, or DNS changes have not propagated yet.

**Fix:**

1. Verify the A record is set correctly with your DNS provider.
2. Check DNS resolution from the server:

   ```bash
   dig +short your.domain.com
   ```

3. Compare the result with the server's public IP:

   ```bash
   curl -s https://ifconfig.me
   ```

4. If the A record was recently created, wait up to 10 minutes for propagation and try again.

### Port Already in Use

**Symptom:** Caddy fails to start with "address already in use" errors on port 80 or 443.

**Cause:** Another web server (Apache, Nginx, or another process) is already bound to ports 80 or 443.

**Fix:**

1. Identify what is using the port:

   ```bash
   ss -tlnp | grep ':80\|:443'
   ```

2. Stop the conflicting service:

   ```bash
   systemctl stop apache2
   systemctl disable apache2
   # or
   systemctl stop nginx
   systemctl disable nginx
   ```

3. Restart Redoubt:

   ```bash
   cd /opt/redoubt
   docker compose up -d
   ```

### Docker Installation Fails

**Symptom:** The installer fails during Docker installation with package conflict errors.

**Cause:** Old or unofficial Docker packages (`docker.io`, `docker-compose`) conflict with Docker Engine.

**Fix:**

1. Remove old packages:

   ```bash
   apt-get remove -y docker docker-engine docker.io containerd runc docker-compose
   ```

2. Follow the official Docker installation guide for your distribution:
   - [Ubuntu](https://docs.docker.com/engine/install/ubuntu/)
   - [Debian](https://docs.docker.com/engine/install/debian/)

3. Re-run the installer.

### TLS Certificate Not Provisioning

**Symptom:** The site loads with a certificate error, or Caddy logs show ACME challenge failures.

**Cause:** DNS is not resolving to the server, port 80 is blocked, or Caddy cannot reach Let's Encrypt.

**Fix:**

1. Verify DNS resolves correctly (see "DNS Not Resolving" above).

2. Confirm port 80 is open:

   ```bash
   ufw status
   ```

   If port 80 is not listed as ALLOW, add it:

   ```bash
   ufw allow 80/tcp
   ```

3. Check Caddy logs for specific errors:

   ```bash
   docker compose logs caddy | grep -i "acme\|tls\|certificate"
   ```

4. Restart Caddy to retry certificate provisioning:

   ```bash
   docker compose restart caddy
   ```

## Runtime Issues

### Cannot Connect to Voice Channels

**Symptom:** Users can see voice channels but cannot join, or audio/video does not work after joining.

**Cause:** UDP ports required for WebRTC media are blocked by the server firewall or the user's network.

**Fix:**

1. Ensure the LiveKit UDP port range is open on the server:

   ```bash
   ufw allow 60000:60100/udp
   ```

2. Verify the ports are published in Docker:

   ```bash
   docker compose ps livekit
   ```

   The output should show `60000-60100/udp` in the ports column.

3. If users are behind restrictive corporate firewalls that block UDP, LiveKit falls back to TCP via port 7881. Ensure this port is open:

   ```bash
   ufw allow 7881/tcp
   ```

4. Check LiveKit logs for connection errors:

   ```bash
   docker compose logs livekit | grep -i "error\|failed"
   ```

### WebSocket Disconnects

**Symptom:** Users get disconnected from chat or see "reconnecting" messages frequently.

**Cause:** A proxy timeout or network issue is dropping the WebSocket connection.

**Fix:**

1. Check Caddy logs for upstream errors:

   ```bash
   docker compose logs caddy | grep -i "error\|timeout\|502"
   ```

2. Verify the API server is running and healthy:

   ```bash
   docker compose ps redoubt-api
   curl -k https://localhost/health
   ```

3. If using an external load balancer or CDN in front of Caddy, ensure it supports WebSocket connections and has appropriate timeout settings (at least 60 seconds).

### Out of Memory

**Symptom:** Services crash and restart repeatedly. `docker compose ps` shows containers in "restarting" state.

**Cause:** The server does not have enough RAM for all services, especially with many concurrent voice/video participants.

**Fix:**

1. Check available memory:

   ```bash
   free -h
   ```

2. Check which container is using the most memory:

   ```bash
   docker stats --no-stream
   ```

3. Options to reduce memory usage:
   - Lower `voice.default_max_participants` in `config/config.yaml`
   - Disable monitoring if enabled: `docker compose --profile monitoring down`
   - Upgrade the server to have more RAM (8 GB recommended for active instances)

### Database Connection Errors

**Symptom:** API logs show "connection refused" or "too many connections" errors for PostgreSQL.

**Cause:** PostgreSQL has crashed (often due to disk space), or the connection pool is exhausted.

**Fix:**

1. Check PostgreSQL status:

   ```bash
   docker compose ps postgres
   ```

2. Check disk space (PostgreSQL will shut down if the disk is full):

   ```bash
   df -h
   ```

3. Check PostgreSQL logs:

   ```bash
   docker compose logs postgres | tail -50
   ```

4. If disk is full, free space and restart:

   ```bash
   docker system prune -f    # Remove unused Docker resources
   docker compose restart postgres
   ```

5. If connection pool is exhausted, consider increasing `database.max_open_conns` in `config/config.yaml` (default: 25).

### LiveKit Not Starting

**Symptom:** LiveKit container keeps restarting or stays unhealthy. Voice channels do not work.

**Cause:** Redis is not ready when LiveKit starts, or the `LIVEKIT_KEYS` environment variable has a formatting issue.

**Fix:**

1. Check LiveKit logs:

   ```bash
   docker compose logs livekit
   ```

2. Verify Redis is healthy:

   ```bash
   docker compose ps redis
   docker compose exec redis redis-cli ping
   ```

3. Verify `LIVEKIT_KEYS` is set correctly. The format must be `key:secret` with no spaces:

   ```bash
   docker compose exec livekit env | grep LIVEKIT_KEYS
   ```

4. If the keys look wrong, check `.env` for `LIVEKIT_API_KEY` and `LIVEKIT_API_SECRET` and ensure they contain no whitespace or newline characters.

5. Restart LiveKit:

   ```bash
   docker compose restart livekit
   ```

## Viewing Logs

Docker Compose provides centralized log access for all services.

```bash
# All services
docker compose logs -f

# API server only
docker compose logs -f redoubt-api

# LiveKit (voice/video)
docker compose logs -f livekit

# Caddy (reverse proxy and TLS)
docker compose logs -f caddy

# PostgreSQL (database)
docker compose logs -f postgres

# Redis
docker compose logs -f redis
```

To view only the last 100 lines:

```bash
docker compose logs --tail 100 redoubt-api
```

To view logs since a specific time:

```bash
docker compose logs --since "2h" redoubt-api
```

## Checking Service Health

### Container Status

```bash
docker compose ps
```

All services should show a status of "Up" or "Up (healthy)". A "Restarting" status indicates a problem -- check the logs for that service.

### Health Endpoint

```bash
curl -k https://localhost/health
```

A healthy response returns HTTP 200. If this fails, the API server or Caddy may be down.

### Individual Service Checks

```bash
# PostgreSQL
docker compose exec postgres pg_isready -U redoubt

# Redis
docker compose exec redis redis-cli ping

# LiveKit
curl -s http://localhost:7880 > /dev/null && echo "LiveKit OK" || echo "LiveKit down"
```

## Common Fixes

### Restart All Services

```bash
cd /opt/redoubt
docker compose restart
```

### Apply Configuration Changes

After editing `.env` or `config/config.yaml`:

```bash
cd /opt/redoubt
docker compose up -d
```

Docker Compose detects configuration changes and recreates only the affected containers.

### Check Disk Space

```bash
df -h
```

If Docker is consuming excessive disk space:

```bash
docker system prune -f           # Remove stopped containers, unused networks, dangling images
docker volume prune -f           # Remove unused volumes (be careful -- do not remove data volumes)
```

### Check Memory Usage

```bash
free -h
docker stats --no-stream
```

### Force Recreate All Containers

If services are in a bad state:

```bash
cd /opt/redoubt
docker compose down
docker compose up -d
```

## Resetting the Instance

> **Warning:** This permanently destroys all data including messages, user accounts, files, and configuration. This action cannot be undone.

To completely reset and start fresh:

```bash
cd /opt/redoubt
docker compose down -v
rm .env
```

Then re-run the installer to set up a new instance:

```bash
curl -fsSL https://raw.githubusercontent.com/redoubtapp/redoubt-api/main/install.sh | bash
```
