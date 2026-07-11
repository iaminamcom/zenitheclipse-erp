# Reset the Admin Password on Dokploy

Use this procedure if you forget the `admin` password for a deployed Zenith Eclipse ERP instance.

## Reset procedure

1. In Dokploy, open the Zenith ERP project and its deployed Compose service.
2. Open the **Terminal** tab for the `zenith-erp` container.
3. Create a safety backup of the ERP data file:

   ```sh
   cp /data/erp_data.json /data/erp_data.json.password-backup
   ```

4. Reset the `admin` password hash:

   ```sh
   sed -i '/"username": "admin"/,/}/ s/"passwordHash": "[^"]*"/"passwordHash": "e501ec64514049741a2fb04bf34c72db3533cda9c67d47af77069ba5847ef0cd"/' /data/erp_data.json
   ```

5. Return to Dokploy and restart the `zenith-erp` service/container.
6. Sign in with the temporary credentials:

   ```text
   Username: admin
   Password: ChangeMe123!
   ```

7. Immediately open **Users & Roles -> Change My Password** and choose a new password.
8. Sign out and confirm that the new password works.

The new password must be at least 10 characters and include an uppercase letter, a lowercase letter, a number, and a symbol.

## Safety notes

- Do not delete the `zenith_erp_data` Docker volume. It contains the deployed ERP data.
- The reset does not remove business records or settings; it changes only the `admin` password hash.
- Keep `/data/erp_data.json.password-backup` until the new login has been verified.
- Restrict access to the Dokploy terminal because it provides direct access to application data.

## Restore the safety backup

If the data file was accidentally damaged, open the container terminal and run:

```sh
cp /data/erp_data.json.password-backup /data/erp_data.json
```

Then restart the `zenith-erp` service in Dokploy.
