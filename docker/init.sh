#!/bin/sh
set -e

echo "==================================="
echo "Payment Gateway Initialization"
echo "==================================="

echo "Waiting for PostgreSQL..."

# Wait for postgres
until pg_isready -h "${GATEWAY_DATABASE__HOST}" -p "${GATEWAY_DATABASE__PORT}" -U "${GATEWAY_DATABASE__USER}"; do
  echo "Waiting for database connection..."
  sleep 2
done

echo "PostgreSQL is ready!"

echo "Creating database if not exists..."
PGPASSWORD="${GATEWAY_DATABASE__PASSWORD}" \
psql -h "${GATEWAY_DATABASE__HOST}" -p "${GATEWAY_DATABASE__PORT}" -U "${GATEWAY_DATABASE__USER}" -d postgres \
  -tc "SELECT 1 FROM pg_database WHERE datname = '${GATEWAY_DATABASE__NAME}'" | grep -q 1 || \
PGPASSWORD="${GATEWAY_DATABASE__PASSWORD}" \
psql -h "${GATEWAY_DATABASE__HOST}" -p "${GATEWAY_DATABASE__PORT}" -U "${GATEWAY_DATABASE__USER}" -d postgres \
  -c "CREATE DATABASE ${GATEWAY_DATABASE__NAME}"

echo "Database ready!"

echo "Running migrations..."
for migration_file in /app/internal/db/migrations/*.sql; do
  [ -f "$migration_file" ] || continue
  echo "Applying migration: $(basename "$migration_file")"
  PGPASSWORD="${GATEWAY_DATABASE__PASSWORD}" \
  psql -h "${GATEWAY_DATABASE__HOST}" -p "${GATEWAY_DATABASE__PORT}" \
       -U "${GATEWAY_DATABASE__USER}" -d "${GATEWAY_DATABASE__NAME}" \
       -f "$migration_file"
done

echo "Migrations complete!"
echo "==================================="
echo "Starting Payment Gateway..."
echo "==================================="

exec ./gateway