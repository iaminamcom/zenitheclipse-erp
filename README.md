# Zenith Eclipse ERP Docker / Dokploy Package

This package contains a Dockerfile and Docker Compose setup for deploying Zenith Eclipse ERP on Dokploy.

## Quick local test

```bash
cp .env.example .env
docker compose -f docker-compose.yml -f docker-compose.local.yml up -d --build
open http://localhost:8080
```

Login:

- Username: `admin`
- Password: `ChangeMe123!`

Change the password after first login.

## Dokploy

Use `docker-compose.yml` with Dokploy Docker Compose. Add a domain in Dokploy and point it to service `zenith-erp` on port `8080`.

Persistent ERP data is stored in the Docker volume `zenith_erp_data` mounted to `/data`.

The container initializes ownership of `/data` automatically, then runs the ERP
as the unprivileged `zenith` user (UID `10001`). After updating an existing
deployment, rebuild the image rather than only restarting the old container.

```bash
docker compose up -d --build --force-recreate
```
