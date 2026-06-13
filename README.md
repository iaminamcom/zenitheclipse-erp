# Zenith Eclipse ERP Ultimate

Elegant, server-ready business management software for Zenith Eclipse Co.

## What is new in this Ultimate build

- Product, Transportation and Service selection on every document.
- A combined document mode for Product + Transportation in one quotation, PI, commercial invoice or packing list.
- Category totals: Products, Transportation, Services/Charges, Subtotal, Tax and Total.
- A more elegant dashboard, sidebar, cards, tables, modals and letterhead preview.
- Catalog items are grouped as product goods, transport/freight/truck/container and service/customs/documentation.
- Print letterheads keep the Zenith structure: logo top-left, document serial/date top-right, To block and footer contact details.

## Main features

- One Serial Business Chain: quotation -> PI -> commercial invoice -> packing list -> agreement, all connected under one base serial.
- Letterhead Designer for company logo, footer, stamp text and document design.
- Customers, suppliers, products, transportation charges, services, HS codes and warehouse stock.
- Quotations, proforma invoices, commercial invoices, packing lists, agreements, delivery notes, purchase orders and vouchers.
- Accounting receipts/payments, expenses, cash/bank accounts, multi-currency fields and customer/supplier balances.
- Shipment/logistics fields: BL, container, seal, POL/POD, vessel, voyage, truck/driver, gross/net weight and packages.
- Reports: sales, unpaid/partial invoices, profit estimate, tax summary, aging and activity/audit logs.
- Employee self-registration with admin approval and role control.
- Backup/restore, CSV export, Excel export and print/save-as-PDF from the browser.
- Server mode so employees can open the same system from their own accounts on phone or computer.
- Android Studio source project for building an APK that opens your server.

## First login

Username: `admin`  
Password: `admin123`

Change the password from Settings after the first login.

## Windows desktop

Double-click `ZenithEclipseERP_Ultimate.exe`. It starts the ERP locally and opens the browser.

## Windows server/LAN mode

Open Command Prompt in the same folder as the EXE and run:

```bat
set ZENITH_ERP_ADDR=0.0.0.0:8080
set ZENITH_ERP_BROWSER=0
set ZENITH_ERP_DATA=C:\ZenithERPData
ZenithEclipseERP_Ultimate.exe
```

Employees open:

```text
http://YOUR-SERVER-IP:8080
```

## Linux server mode

```bash
mkdir -p /var/lib/zenith-erp
chmod +x ZenithEclipseERP_Ultimate_Server_Linux
ZENITH_ERP_ADDR=0.0.0.0:8080 ZENITH_ERP_BROWSER=0 ZENITH_ERP_DATA=/var/lib/zenith-erp ./ZenithEclipseERP_Ultimate_Server_Linux
```

## Employee accounts

1. Employee opens your ERP URL.
2. Employee clicks **Create employee account**.
3. Admin logs in with the admin account.
4. Admin opens **Employees & Access**.
5. Admin approves the employee and chooses the role.

## Important for public internet use

Use HTTPS before exposing it on the internet. Put the app behind Nginx, Caddy, Cloudflare Tunnel, or your hosting provider's SSL system. Keep strong passwords and regular backups.
