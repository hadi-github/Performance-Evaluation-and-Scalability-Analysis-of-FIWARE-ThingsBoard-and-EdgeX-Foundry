#!/bin/bash
set -e

echo "Waiting for primary database to be ready..."
until pg_isready -h "$POSTGRES_PRIMARY_HOST" -p "$POSTGRES_PRIMARY_PORT" -U replicator; do
  sleep 2
done

echo "Primary is ready. Starting base backup..."

# Remove any existing data
rm -rf /var/lib/postgresql/data/*

# Set password for pg_basebackup
export PGPASSWORD="$POSTGRES_REPLICATION_PASSWORD"

# Create base backup from primary
pg_basebackup -h "$POSTGRES_PRIMARY_HOST" -D /var/lib/postgresql/data -U replicator -v -P --wal-method=stream

# Create recovery configuration
cat > /var/lib/postgresql/data/postgresql.auto.conf <<EOF2
primary_conninfo = 'host=$POSTGRES_PRIMARY_HOST port=$POSTGRES_PRIMARY_PORT user=replicator password=$POSTGRES_REPLICATION_PASSWORD'
hot_standby = on
EOF2

# Create standby signal file
touch /var/lib/postgresql/data/standby.signal

echo "Replica setup complete. Starting PostgreSQL..."
