# Zenith Eclipse ERP Ultimate - Docker + Dokploy Deployment Guide

This package contains the Docker-ready version of Zenith Eclipse ERP Ultimate for online deployment.

The application listens on port **8080** inside the container and stores its database in:

```text
/data/data.json
```

Keep `/data` mounted as a persistent volume. Do not deploy multiple replicas of this version against the same JSON file. For scaling to many replicas later, convert the database layer to PostgreSQL first.

## Files included

```text
Dockerfile                              Build the production Docker image
docker-compose.yml                      Local testing with localhost:8080
docker-compose.dokploy.yml              Recommended Dokploy Compose file
docker-compose.dokploy.manual-traefik.yml Advanced manual Traefik option
docker-compose.dokploy.prebuilt-amd64.yml Use after docker load on AMD64 server
docker-compose.dokploy.prebuilt-arm64.yml Use after docker load on ARM64 server
.env.example                            Environment variable template
build-docker.sh                         Local image build helper
push-to-registry.sh                     Multi-arch push helper
load-prebuilt-image.sh                  Load the supplied tar image on server
main.go / web/                          Application source
```

## Login after first deployment

Default username:

```text
admin
```

The admin password comes from:

```text
ZENITH_ERP_ADMIN_PASSWORD
```

Important: this variable is only used the first time the `/data/data.json` file is created. After that, change passwords inside the app or delete/restore the data file intentionally.

## Recommended Dokploy method: deploy from GitHub/Git

### 1. Prepare the project repository

Unzip the Docker package on your computer.

```bash
unzip ZenithEclipseERP_Docker_Dokploy_Package.zip
cd ZenithEclipseERP_Docker_Dokploy
```

Create a private GitHub repository, then upload/push all files from this folder.

### 2. Edit the Dokploy compose password

Open:

```text
docker-compose.dokploy.yml
```

Change:

```yaml
ZENITH_ERP_ADMIN_PASSWORD: "CHANGE_ME_BEFORE_FIRST_DEPLOY"
```

to a strong password.

### 3. Create the service in Dokploy

In Dokploy:

1. Create or open your project.
2. Add a new **Compose** service.
3. Choose **Docker Compose**.
4. Select your Git provider and repository.
5. Branch: `main` or your production branch.
6. Compose path: `./docker-compose.dokploy.yml`.
7. Save and deploy.

### 4. Add domain in Dokploy

In your Compose service:

1. Go to **Domains**.
2. Add your domain, for example:

```text
erp.zenitheclipse.com
```

3. Select service:

```text
zenith-erp
```

4. Select port:

```text
8080
```

5. Enable HTTPS/SSL.
6. Redeploy after adding the domain.

Make sure your DNS **A record** points to your Dokploy server IP before creating SSL.

### 5. Open the app

Open:

```text
https://erp.yourdomain.com
```

Login as `admin`, then change the password inside the app.

## Alternative method: deploy prebuilt Docker image tar

Use this if you do not want Dokploy to build from source.

### 1. Copy image tar to the Dokploy server

For most Ubuntu VPS servers:

```bash
scp ZenithEclipseERP_Docker_Image_amd64.tar root@YOUR_SERVER_IP:/root/
```

For ARM servers:

```bash
scp ZenithEclipseERP_Docker_Image_arm64.tar root@YOUR_SERVER_IP:/root/
```

### 2. Load the image on the server

SSH into your server:

```bash
ssh root@YOUR_SERVER_IP
cd /root
sudo docker load -i ZenithEclipseERP_Docker_Image_amd64.tar
sudo docker images | grep zenitheclipse
```

### 3. Use prebuilt compose file in Dokploy

Upload this package to Git, then in Dokploy use one of these compose paths:

```text
./docker-compose.dokploy.prebuilt-amd64.yml
```

or:

```text
./docker-compose.dokploy.prebuilt-arm64.yml
```

Then add the domain with service `zenith-erp` and port `8080`.

## Local test before Dokploy

```bash
docker compose up -d --build
curl http://127.0.0.1:8080/healthz
```

Open:

```text
http://127.0.0.1:8080
```

Stop:

```bash
docker compose down
```

Stop and delete local data volume:

```bash
docker compose down -v
```

## Backup

The important file is:

```text
/data/data.json
```

From the server, find the container name:

```bash
docker ps | grep zenith
```

Create a backup inside the volume:

```bash
docker exec -it CONTAINER_NAME sh -c 'cp /data/data.json /data/data-$(date +%F-%H%M).json'
```

Copy a backup out to the server:

```bash
docker cp CONTAINER_NAME:/data/data.json ./zenith-data-backup.json
```

Keep backups outside the VPS too.

## Update procedure

1. Backup `/data/data.json`.
2. Push new source changes to Git or upload a new image.
3. Redeploy in Dokploy.
4. Check `/healthz` and login.

Persistent data stays in the named volume `zenith_erp_data`.

## Troubleshooting

### Domain gives Bad Gateway

Check these first:

- The app must listen on `0.0.0.0:8080`.
- Dokploy domain port must be `8080`.
- In Dokploy, do not map host port `8080:8080`; use the compose file as provided.
- Redeploy after adding or editing the domain.

### Login cookie problem after HTTPS

Set:

```text
ZENITH_ERP_COOKIE_SECURE=1
```

For local HTTP testing only, use:

```text
ZENITH_ERP_COOKIE_SECURE=0
```

### Password environment variable did not change admin password

`ZENITH_ERP_ADMIN_PASSWORD` only works before the first database is created. Once `/data/data.json` exists, change the password inside the app or reset it from the admin panel.

## Production notes

- Keep employee self-registration enabled only if admin approval is required.
- Do not scale this container to 2+ replicas while it uses JSON storage.
- Put the service behind HTTPS in Dokploy.
- Backup before every update.
- For high-volume company use, the next upgrade should be PostgreSQL storage and automatic scheduled backups.
