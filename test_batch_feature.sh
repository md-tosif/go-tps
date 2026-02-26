#!/bin/bash

# Test script to verify batch number functionality

echo "=== Batch Number Feature Test ==="
echo ""
echo "This script demonstrates the batch number tracking feature."
echo ""

DB_FILE="test_batch.db"

# Clean up any existing test database
if [ -f "$DB_FILE" ]; then
    echo "Removing existing test database..."
    rm "$DB_FILE"
fi

echo "Running first test execution..."
echo "--------------------------------"
RUN_DURATION_MINUTES=0 \
DB_PATH="$DB_FILE" \
WALLET_COUNT=1 \
TX_PER_WALLET=2 \
./go-tps

echo ""
echo "Waiting 3 seconds before second execution..."
sleep 3

echo ""
echo "Running second test execution..."
echo "--------------------------------"
RUN_DURATION_MINUTES=0 \
DB_PATH="$DB_FILE" \
WALLET_COUNT=1 \
TX_PER_WALLET=2 \
./go-tps

echo ""
echo ""
echo "=== Verifying Batch Numbers in Database ==="
echo ""

# Check that we have multiple batches
BATCH_COUNT=$(sqlite3 "$DB_FILE" "SELECT COUNT(DISTINCT batch_number) FROM transactions;")
echo "Total batches in database: $BATCH_COUNT"

if [ "$BATCH_COUNT" -ge 2 ]; then
    echo "✓ Multiple batches detected successfully!"
else
    echo "✗ Expected at least 2 batches, found $BATCH_COUNT"
fi

echo ""
echo "Listing all batches:"
echo "--------------------"
sqlite3 "$DB_FILE" -header -column "SELECT batch_number, COUNT(*) as tx_count FROM transactions GROUP BY batch_number ORDER BY batch_number;"

echo ""
echo "=== Batch Analysis ==="
echo ""
DB_PATH="$DB_FILE" ./analyze.sh batches

echo ""
echo "Test complete! Database saved as: $DB_FILE"
echo "You can query it with: sqlite3 $DB_FILE"
echo "Or analyze it with: DB_PATH=$DB_FILE ./analyze.sh batches"
