#!/usr/bin/env bash
set -e
mkdir -p ./data
export ZENITH_ERP_ADDR=0.0.0.0:8080
export ZENITH_ERP_BROWSER=0
export ZENITH_ERP_DATA=$(pwd)/data
chmod +x ./ZenithEclipseERP_Ultimate_Server_Linux
./ZenithEclipseERP_Ultimate_Server_Linux
