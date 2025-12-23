#!/bin/bash
set -e

echo "Starting Payment Gateway in Production Mode"
cd "$(dirname "$0")"
docker-compose -f docker-compose.yml up --build -d  