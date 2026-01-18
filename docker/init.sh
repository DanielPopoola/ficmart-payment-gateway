#!/bin/sh
set -e

# Wait for DB
until pg_isready -h "$GATEWAY_DATABASE__HOST" -U "$GATEWAY_DATABASE__USER"; do
  echo "Waiting for DB..."
  sleep 2
done


echo "Running migrations..."
migrate -path ./internal/db/migrations \
  -database "postgres://${GATEWAY_DATABASE__USER}:${GATEWAY_DATABASE__PASSWORD}@${GATEWAY_DATABASE__HOST}:${GATEWAY_DATABASE__PORT}/${GATEWAY_DATABASE__NAME}?sslmode=${GATEWAY_DATABASE__SSL_MODE}" \
  up
  
exec "$@"