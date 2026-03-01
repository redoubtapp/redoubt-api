# Upgrade Guide

Redoubt is distributed as a Docker image. Upgrading pulls the latest image and restarts services. Database migrations run automatically on startup.

## Standard Upgrade

```bash
cd /opt/redoubt
docker compose pull
docker compose up -d
```

This pulls the latest `ghcr.io/redoubtapp/redoubt-api:latest` image and recreates only the containers that have changed. Data volumes are preserved.

## Upgrade with Monitoring

If you have the monitoring profile enabled, include it in both commands:

```bash
cd /opt/redoubt
docker compose --profile monitoring pull
docker compose --profile monitoring up -d
```

## Using the Installer

The installer script detects existing installations and automatically runs the upgrade flow instead of a fresh install:

```bash
curl -fsSL https://raw.githubusercontent.com/redoubtapp/redoubt-api/main/install.sh | bash
```

The upgrade flow pulls the latest images, updates configuration files if needed, and restarts services. Existing secrets and data are preserved.

## Checking the Current Version

The API logs its version on startup. To check which version is running:

```bash
docker compose logs redoubt-api | grep "starting redoubt-api" | tail -1
```

## Pinning a Specific Version

By default, the compose file uses `ghcr.io/redoubtapp/redoubt-api:latest`. To pin a specific version, edit `docker-compose.yml` and replace the image tag:

```yaml
services:
  redoubt-api:
    image: ghcr.io/redoubtapp/redoubt-api:1.0.0
```

Then apply the change:

```bash
cd /opt/redoubt
docker compose pull
docker compose up -d
```

> **Note:** When pinned to a specific version, `docker compose pull` will only pull that exact tag. You will need to manually update the tag in `docker-compose.yml` for future upgrades.

## Rolling Back

If an upgrade introduces issues, roll back to a previous version:

1. Stop the current services:

   ```bash
   cd /opt/redoubt
   docker compose down
   ```

2. Edit `docker-compose.yml` and set the image tag to the previous working version:

   ```yaml
   services:
     redoubt-api:
       image: ghcr.io/redoubtapp/redoubt-api:1.0.0
   ```

3. Pull and start the pinned version:

   ```bash
   docker compose pull
   docker compose up -d
   ```

4. Verify the rollback:

   ```bash
   docker compose logs redoubt-api | grep "starting redoubt-api" | tail -1
   ```

> **Note:** Database migrations are forward-only. If a new version introduced schema changes, rolling back the application may cause errors if the new schema is incompatible with the older code. Check the release notes before rolling back.

## Database Migrations

Migrations run automatically when the API server starts. No manual migration commands are needed.

The API applies any pending migrations before accepting traffic. If a migration fails, the server will exit with an error. Check the logs:

```bash
docker compose logs redoubt-api | grep -i migration
```

## Breaking Changes

No breaking changes have been introduced yet. This section will be updated with version-specific migration notes as the project evolves.

When breaking changes do occur, they will be documented here with:

- The version that introduced the change
- What changed and why
- Required manual steps (if any)
- Configuration changes needed
