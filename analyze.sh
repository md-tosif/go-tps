#!/bin/bash

# Performance Analysis Script for go-tps
# This script provides easy access to common database queries

DB_PATH="${DB_PATH:-./transactions2.db}"

if [ ! -f "$DB_PATH" ]; then
    echo "Error: Database file not found at $DB_PATH"
    echo "Run go-tps first to generate transaction data"
    exit 1
fi

show_help() {
    cat << EOF
Performance Analysis Script for go-tps

Usage: $0 [command]

Commands:
    summary         Show overall summary statistics
    tps             Show TPS (Transactions Per Second) metrics
    performance     Show detailed performance metrics
    wallets         Show per-wallet statistics
    batches         List all batches and show batch statistics
    batch <id>      Show statistics for a specific batch
    recent          Show recent transactions
    errors          Show error analysis
    timeline        Show transactions over time
    export          Export data to CSV
    query           Open interactive SQL shell
    help            Show this help message

Examples:
    $0 summary
    $0 tps
    $0 performance
    $0 wallets
    $0 batches
    $0 batch batch-20260226-143025
    
Environment Variables:
    DB_PATH         Path to database file (default: ./transactions.db)

EOF
}

summary() {
    echo "=== TRANSACTION SUMMARY ==="
    sqlite3 "$DB_PATH" <<EOF
SELECT 
    'Total Transactions: ' || COUNT(*) as stat
FROM transactions
UNION ALL
SELECT 
    'Successful: ' || SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END)
FROM transactions
UNION ALL
SELECT 
    'Failed: ' || SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END)
FROM transactions
UNION ALL
SELECT 
    'Pending: ' || SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END)
FROM transactions
UNION ALL
SELECT 
    'Unique Wallets: ' || COUNT(DISTINCT wallet_address)
FROM transactions
UNION ALL
SELECT 
    'Avg Execution Time: ' || ROUND(AVG(execution_time), 2) || ' ms'
FROM transactions
WHERE execution_time > 0;
EOF
}

performance() {
    echo "=== PERFORMANCE METRICS ==="
    sqlite3 "$DB_PATH" -header -column <<EOF
SELECT 
    status,
    COUNT(*) as count,
    ROUND(AVG(execution_time), 2) as avg_ms,
    ROUND(MIN(execution_time), 2) as min_ms,
    ROUND(MAX(execution_time), 2) as max_ms
FROM transactions
WHERE execution_time > 0
GROUP BY status;
EOF
}

wallets() {
    echo "=== WALLET STATISTICS ==="
    sqlite3 "$DB_PATH" -header -column <<EOF
SELECT 
    SUBSTR(wallet_address, 1, 10) || '...' as wallet,
    COUNT(*) as total,
    SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as success,
    SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed,
    ROUND(AVG(execution_time), 2) as avg_ms
FROM transactions
GROUP BY wallet_address
ORDER BY total DESC;
EOF
}

recent() {
    echo "=== RECENT TRANSACTIONS (Last 10) ==="
    sqlite3 "$DB_PATH" -header -column <<EOF
SELECT 
    SUBSTR(wallet_address, 1, 10) || '...' as wallet,
    SUBSTR(tx_hash, 1, 12) || '...' as tx_hash,
    nonce,
    status,
    ROUND(execution_time, 2) as time_ms,
    submitted_at
FROM transactions
ORDER BY submitted_at DESC
LIMIT 10;
EOF
}

errors() {
    echo "=== ERROR ANALYSIS ==="
    sqlite3 "$DB_PATH" -header -column <<EOF
SELECT 
    error,
    COUNT(*) as count
FROM transactions
WHERE error IS NOT NULL AND error != ''
GROUP BY error
ORDER BY count DESC
LIMIT 10;
EOF
    
    echo ""
    echo "=== RECENT FAILED TRANSACTIONS ==="
    sqlite3 "$DB_PATH" -header -column <<EOF
SELECT 
    SUBSTR(wallet_address, 1, 10) || '...' as wallet,
    nonce,
    SUBSTR(error, 1, 50) as error_msg,
    submitted_at
FROM transactions
WHERE status = 'failed'
ORDER BY submitted_at DESC
LIMIT 5;
EOF
}

timeline() {
    echo "=== TRANSACTIONS PER SECOND ==="
    sqlite3 "$DB_PATH" -header -column <<EOF
SELECT 
    strftime('%H:%M:%S', submitted_at) as time,
    COUNT(*) as tps,
    ROUND(AVG(execution_time), 2) as avg_ms
FROM transactions
GROUP BY strftime('%Y-%m-%d %H:%M:%S', submitted_at)
ORDER BY time DESC
LIMIT 20;
EOF
}

export_data() {
    OUTPUT_FILE="transactions_export_$(date +%Y%m%d_%H%M%S).csv"
    echo "Exporting to $OUTPUT_FILE..."
    
    sqlite3 "$DB_PATH" -header -csv <<EOF > "$OUTPUT_FILE"
SELECT 
    wallet_address,
    tx_hash,
    nonce,
    to_address,
    value,
    gas_price,
    gas_limit,
    status,
    submitted_at,
    confirmed_at,
    execution_time,
    error
FROM transactions
ORDER BY submitted_at;
EOF
    
    echo "✓ Data exported to $OUTPUT_FILE"
    
    # Also create a summary CSV
    SUMMARY_FILE="summary_$(date +%Y%m%d_%H%M%S).csv"
    echo "Creating summary in $SUMMARY_FILE..."
    
    sqlite3 "$DB_PATH" -header -csv <<EOF > "$SUMMARY_FILE"
SELECT 
    wallet_address,
    COUNT(*) as total_tx,
    SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as successful,
    SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed,
    ROUND(AVG(execution_time), 2) as avg_time_ms,
    MIN(nonce) as first_nonce,
    MAX(nonce) as last_nonce
FROM transactions
GROUP BY wallet_address;
EOF
    
    echo "✓ Summary exported to $SUMMARY_FILE"
}

tps() {
    echo "=== TPS (Transactions Per Second) METRICS ==="
    sqlite3 "$DB_PATH" <<EOF
.mode column
.headers on

WITH time_range AS (
    SELECT 
        MIN(submitted_at) as min_submit,
        MAX(submitted_at) as max_submit,
        MIN(confirmed_at) as min_confirm,
        MAX(confirmed_at) as max_confirm,
        COUNT(*) as total_tx,
        SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as success_tx
    FROM transactions
)
SELECT 
    '=== Submission TPS ===' as metric,
    '' as value
UNION ALL
SELECT 
    'Total Transactions:',
    CAST(total_tx as TEXT)
FROM time_range
UNION ALL
SELECT 
    'Time Window:',
    ROUND((JULIANDAY(max_submit) - JULIANDAY(min_submit)) * 86400, 2) || ' seconds'
FROM time_range
UNION ALL
SELECT 
    'TPS (Submission):',
    CAST(ROUND(CAST(total_tx as REAL) / 
        ((JULIANDAY(max_submit) - JULIANDAY(min_submit)) * 86400), 2) as TEXT) || ' tx/s'
FROM time_range
WHERE (JULIANDAY(max_submit) - JULIANDAY(min_submit)) * 86400 > 0
UNION ALL
SELECT '', ''
UNION ALL
SELECT 
    '=== Confirmation TPS ===',
    ''
UNION ALL
SELECT 
    'Successful Transactions:',
    CAST(success_tx as TEXT)
FROM time_range
UNION ALL
SELECT 
    'Time Window:',
    ROUND((JULIANDAY(max_confirm) - JULIANDAY(min_confirm)) * 86400, 2) || ' seconds'
FROM time_range
WHERE max_confirm IS NOT NULL
UNION ALL
SELECT 
    'TPS (Confirmation):',
    CAST(ROUND(CAST(success_tx as REAL) / 
        ((JULIANDAY(max_confirm) - JULIANDAY(min_confirm)) * 86400), 2) as TEXT) || ' tx/s'
FROM time_range
WHERE max_confirm IS NOT NULL AND (JULIANDAY(max_confirm) - JULIANDAY(min_confirm)) * 86400 > 0;
EOF

    echo ""
    echo "=== TPS Over Time (per second) ==="
    sqlite3 "$DB_PATH" -header -column <<EOF
SELECT 
    strftime('%H:%M:%S', submitted_at) as second,
    COUNT(*) as tx_count,
    ROUND(AVG(execution_time), 2) as avg_time_ms
FROM transactions
WHERE status = 'success'
GROUP BY strftime('%Y-%m-%d %H:%M:%S', submitted_at)
ORDER BY second DESC
LIMIT 10;
EOF
}

batches() {
    echo "=== ALL BATCHES ==="
    sqlite3 "$DB_PATH" -header -column <<EOF
SELECT 
    batch_number,
    COUNT(*) as total_tx,
    SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as success,
    SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed,
    SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) as pending,
    ROUND(AVG(execution_time), 2) as avg_ms,
    MIN(submitted_at) as started,
    MAX(submitted_at) as completed
FROM transactions
GROUP BY batch_number
ORDER BY batch_number DESC;
EOF
}

batch_stats() {
    BATCH_ID="$1"
    
    if [ -z "$BATCH_ID" ]; then
        echo "Error: Please specify a batch ID"
        echo "Usage: $0 batch <batch_id>"
        echo ""
        echo "Available batches:"
        sqlite3 "$DB_PATH" "SELECT DISTINCT batch_number FROM transactions ORDER BY batch_number DESC;"
        exit 1
    fi
    
    # Check if batch exists
    COUNT=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM transactions WHERE batch_number = '$BATCH_ID';")
    if [ "$COUNT" -eq 0 ]; then
        echo "Error: Batch '$BATCH_ID' not found"
        echo ""
        echo "Available batches:"
        sqlite3 "$DB_PATH" "SELECT DISTINCT batch_number FROM transactions ORDER BY batch_number DESC;"
        exit 1
    fi
    
    echo "=== BATCH: $BATCH_ID ==="
    echo ""
    
    echo "--- Summary ---"
    sqlite3 "$DB_PATH" <<EOF
SELECT 
    'Total Transactions: ' || COUNT(*)
FROM transactions
WHERE batch_number = '$BATCH_ID'
UNION ALL
SELECT 
    'Successful: ' || SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END)
FROM transactions
WHERE batch_number = '$BATCH_ID'
UNION ALL
SELECT 
    'Failed: ' || SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END)
FROM transactions
WHERE batch_number = '$BATCH_ID'
UNION ALL
SELECT 
    'Pending: ' || SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END)
FROM transactions
WHERE batch_number = '$BATCH_ID'
UNION ALL
SELECT 
    'Avg Execution Time: ' || ROUND(AVG(execution_time), 2) || ' ms'
FROM transactions
WHERE batch_number = '$BATCH_ID' AND execution_time > 0;
EOF

    echo ""
    echo "--- Time Range ---"
    sqlite3 "$DB_PATH" <<EOF
SELECT 
    'Started: ' || MIN(submitted_at)
FROM transactions
WHERE batch_number = '$BATCH_ID'
UNION ALL
SELECT 
    'Completed: ' || MAX(submitted_at)
FROM transactions
WHERE batch_number = '$BATCH_ID'
UNION ALL
SELECT 
    'Duration: ' || ROUND((JULIANDAY(MAX(submitted_at)) - JULIANDAY(MIN(submitted_at))) * 86400, 2) || ' seconds'
FROM transactions
WHERE batch_number = '$BATCH_ID';
EOF

    echo ""
    echo "--- TPS for this Batch ---"
    sqlite3 "$DB_PATH" <<EOF
SELECT 
    'TPS: ' || ROUND(CAST(COUNT(*) as REAL) / 
        ((JULIANDAY(MAX(submitted_at)) - JULIANDAY(MIN(submitted_at))) * 86400), 2) || ' tx/s'
FROM transactions
WHERE batch_number = '$BATCH_ID'
    AND (JULIANDAY(MAX(submitted_at)) - JULIANDAY(MIN(submitted_at))) * 86400 > 0;
EOF

    echo ""
    echo "--- Wallet Statistics for this Batch ---"
    sqlite3 "$DB_PATH" -header -column <<EOF
SELECT 
    SUBSTR(wallet_address, 1, 10) || '...' as wallet,
    COUNT(*) as total,
    SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as success,
    SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed,
    ROUND(AVG(execution_time), 2) as avg_ms
FROM transactions
WHERE batch_number = '$BATCH_ID'
GROUP BY wallet_address
ORDER BY total DESC;
EOF
}

interactive() {
    echo "Opening interactive SQL shell..."
    echo "Database: $DB_PATH"
    echo "Type .help for SQLite help, .quit to exit"
    echo "See queries.sql for example queries"
    echo ""
    sqlite3 "$DB_PATH"
}

case "${1:-help}" in
    summary)
        summary
        ;;
    tps)
        tps
        ;;
    performance)
        performance
        ;;
    wallets)
        wallets
        ;;
    batches)
        batches
        ;;
    batch)
        batch_stats "$2"
        ;;
    recent)
        recent
        ;;
    errors)
        errors
        ;;
    timeline)
        timeline
        ;;
    export)
        export_data
        ;;
    query|interactive|sql)
        interactive
        ;;
    help|--help|-h)
        show_help
        ;;
    *)
        echo "Unknown command: $1"
        echo "Run '$0 help' for usage information"
        exit 1
        ;;
esac
