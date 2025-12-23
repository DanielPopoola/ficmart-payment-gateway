#!/bin/sh
set -e

echo "==================================="
echo "Payment Gateway Initialization"
echo "==================================="

# Wait for PostgreSQL to be ready
echo "Waiting for PostgreSQL..."
until PGPASSWORD=$GATEWAY_DATABASE__PASSWORD psql -h "$GATEWAY_DATABASE__HOST" -U "$GATEWAY_DATABASE__USER" -d "postgres" -c '\q' 2>/dev/null; do
  echo "PostgreSQL is unavailable - sleeping"
  sleep 2
done

echo "PostgreSQL is ready!"

# Create database if it doesn't exist
echo "Creating database if not exists..."
PGPASSWORD=$GATEWAY_DATABASE__PASSWORD psql -h "$GATEWAY_DATABASE__HOST" -U "$GATEWAY_DATABASE__USER" -d "postgres" -tc "SELECT 1 FROM pg_database WHERE datname = '$GATEWAY_DATABASE__NAME'" | grep -q 1 || \
PGPASSWORD=$GATEWAY_DATABASE__PASSWORD psql -h "$GATEWAY_DATABASE__HOST" -U "$GATEWAY_DATABASE__USER" -d "postgres" -c "CREATE DATABASE $GATEWAY_DATABASE__NAME"

echo "Database ready!"

# Run migrations
echo "Running migrations..."
for migration_file in /app/migrations/*.sql; do
    if [ -f "$migration_file" ]; then
        echo "Applying migration: $(basename $migration_file)"
        PGPASSWORD=$GATEWAY_DATABASE__PASSWORD psql -h "$GATEWAY_DATABASE__HOST" -U "$GATEWAY_DATABASE__USER" -d "$GATEWAY_DATABASE__NAME" -f "$migration_file"
    fi
done

echo "Migrations complete!"
echo "==================================="
echo "Starting Payment Gateway..."
echo "==================================="

# Start the application
exec ./gateway