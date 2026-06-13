# Zenith Eclipse ERP Ultimate - Server Deployment

## Local office server

Use this when employees are in the same office/Wi-Fi network.

### Windows

```bat
mkdir C:\ZenithERPData
set ZENITH_ERP_ADDR=0.0.0.0:8080
set ZENITH_ERP_BROWSER=0
set ZENITH_ERP_DATA=C:\ZenithERPData
ZenithEclipseERP_Ultimate.exe
```

Find your server IP address with:

```bat
ipconfig
```

Employees open:

```text
http://SERVER-IP:8080
```

### Linux VPS/server

```bash
sudo mkdir -p /var/lib/zenith-erp
sudo chown $USER:$USER /var/lib/zenith-erp
chmod +x ZenithEclipseERP_Ultimate_Server_Linux
ZENITH_ERP_ADDR=0.0.0.0:8080 ZENITH_ERP_BROWSER=0 ZENITH_ERP_DATA=/var/lib/zenith-erp ./ZenithEclipseERP_Ultimate_Server_Linux
```

## Public domain setup

For public access, do not use plain HTTP. Use HTTPS through Nginx, Caddy, Cloudflare, or your hosting dashboard.

Example reverse proxy target:

```text
127.0.0.1:8080
```

## Employee self-registration

Employees open your ERP URL, create an account, then wait for admin approval. Admin approves users from **Employees & Access**.

## Backups

Data is stored in `data.json` inside the folder set by `ZENITH_ERP_DATA`. Use the app's Settings -> Backup before updates and keep copies outside the server.

## Upgrade rule

Before replacing the EXE/server file:

1. Download a backup from Settings.
2. Stop the running ERP.
3. Replace the binary.
4. Start it again with the same `ZENITH_ERP_DATA` folder.
