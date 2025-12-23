#!/bin/bash
set -e

echo "Starting Payment Gateway in Development Mode"
cd "$(dirname "$0")"
docker-compose up --build