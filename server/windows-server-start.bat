@echo off
mkdir C:\ZenithERPData 2>nul
set ZENITH_ERP_ADDR=0.0.0.0:8080
set ZENITH_ERP_BROWSER=0
set ZENITH_ERP_DATA=C:\ZenithERPData
ZenithEclipseERP_Ultimate.exe
pause
