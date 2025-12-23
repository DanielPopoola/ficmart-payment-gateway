#!/bin/bash
set -e

echo "Stopping Payment Gateway"
cd "$(dirname "$0")"
docker-compose down