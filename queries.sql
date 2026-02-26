-- Performance Analysis Queries for go-tps

-- ==============================================
-- BATCH ANALYSIS
-- ==============================================

-- List all batches with summary statistics
SELECT 
    batch_number,
    COUNT(*) as total_tx,
    SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as successful,
    SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed,
    SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) as pending,
    ROUND(AVG(execution_time), 2) as avg_execution_ms,
    MIN(submitted_at) as batch_start,
    MAX(submitted_at) as batch_end,
    ROUND((JULIANDAY(MAX(submitted_at)) - JULIANDAY(MIN(submitted_at))) * 86400, 2) as duration_seconds
FROM transactions
GROUP BY batch_number
ORDER BY batch_number DESC;

-- Get statistics for a specific batch (replace 'BATCH_ID' with actual batch number)
SELECT 
    'Batch: ' || batch_number as info,
    COUNT(*) as total_transactions,
    SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as successful,
    SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed,
    ROUND(AVG(execution_time), 2) as avg_time_ms,
    MIN(submitted_at) as started,
    MAX(submitted_at) as completed
FROM transactions
WHERE batch_number = 'BATCH_ID'
GROUP BY batch_number;

-- Compare TPS across different batches
SELECT 
    batch_number,
    COUNT(*) as tx_count,
    ROUND((JULIANDAY(MAX(submitted_at)) - JULIANDAY(MIN(submitted_at))) * 86400, 2) as duration_sec,
    ROUND(CAST(COUNT(*) as REAL) / 
        ((JULIANDAY(MAX(submitted_at)) - JULIANDAY(MIN(submitted_at))) * 86400), 2) as tps
FROM transactions
GROUP BY batch_number
HAVING duration_sec > 0
ORDER BY tps DESC;

-- Get most recent batch
SELECT batch_number
FROM transactions
ORDER BY id DESC
LIMIT 1;

-- ==============================================
-- BASIC STATISTICS
-- ==============================================

-- Total transaction count
SELECT 
    COUNT(*) as total_transactions,
    COUNT(DISTINCT wallet_address) as unique_wallets
FROM transactions;

-- Transaction status breakdown
SELECT 
    status,
    COUNT(*) as count,
    ROUND(COUNT(*) * 100.0 / (SELECT COUNT(*) FROM transactions), 2) as percentage
FROM transactions
GROUP BY status
ORDER BY count DESC;

-- ==============================================
-- TPS (TRANSACTIONS PER SECOND) METRICS
-- ==============================================

-- Overall TPS based on submission time
SELECT 
    COUNT(*) as total_transactions,
    MIN(submitted_at) as first_tx_time,
    MAX(submitted_at) as last_tx_time,
    ROUND((JULIANDAY(MAX(submitted_at)) - JULIANDAY(MIN(submitted_at))) * 86400, 2) as duration_seconds,
    ROUND(CAST(COUNT(*) as REAL) / ((JULIANDAY(MAX(submitted_at)) - JULIANDAY(MIN(submitted_at))) * 86400), 2) as tps_submission
FROM transactions
WHERE (JULIANDAY(MAX(submitted_at)) - JULIANDAY(MIN(submitted_at))) * 86400 > 0;

-- Overall TPS based on confirmation time
SELECT 
    COUNT(*) as successful_transactions,
    MIN(confirmed_at) as first_confirm_time,
    MAX(confirmed_at) as last_confirm_time,
    ROUND((JULIANDAY(MAX(confirmed_at)) - JULIANDAY(MIN(confirmed_at))) * 86400, 2) as duration_seconds,
    ROUND(CAST(COUNT(*) as REAL) / ((JULIANDAY(MAX(confirmed_at)) - JULIANDAY(MIN(confirmed_at))) * 86400), 2) as tps_confirmation
FROM transactions
WHERE status = 'success' 
    AND confirmed_at IS NOT NULL
    AND (JULIANDAY(MAX(confirmed_at)) - JULIANDAY(MIN(confirmed_at))) * 86400 > 0;

-- TPS per wallet
SELECT 
    wallet_address,
    COUNT(*) as tx_count,
    MIN(submitted_at) as first_tx,
    MAX(submitted_at) as last_tx,
    ROUND((JULIANDAY(MAX(submitted_at)) - JULIANDAY(MIN(submitted_at))) * 86400, 2) as duration_seconds,
    ROUND(CAST(COUNT(*) as REAL) / 
        NULLIF((JULIANDAY(MAX(submitted_at)) - JULIANDAY(MIN(submitted_at))) * 86400, 0), 2) as tps
FROM transactions
GROUP BY wallet_address
ORDER BY tps DESC;

-- TPS over time (per second intervals)
SELECT 
    strftime('%Y-%m-%d %H:%M:%S', submitted_at) as time_second,
    COUNT(*) as tx_in_second,
    COUNT(*) as instantaneous_tps
FROM transactions
GROUP BY strftime('%Y-%m-%d %H:%M:%S', submitted_at)
ORDER BY time_second;

-- TPS over time (5-second intervals)
SELECT 
    strftime('%Y-%m-%d %H:%M', submitted_at) || ':' || 
        CAST((CAST(strftime('%S', submitted_at) as INTEGER) / 5) * 5 as TEXT) as time_5sec,
    COUNT(*) as tx_count,
    ROUND(CAST(COUNT(*) as REAL) / 5.0, 2) as avg_tps
FROM transactions
GROUP BY time_5sec
ORDER BY time_5sec;

-- Peak TPS (highest transactions per second)
SELECT 
    strftime('%Y-%m-%d %H:%M:%S', submitted_at) as peak_second,
    COUNT(*) as tx_count,
    'Peak TPS' as metric
FROM transactions
GROUP BY strftime('%Y-%m-%d %H:%M:%S', submitted_at)
ORDER BY tx_count DESC
LIMIT 1;

-- ==============================================
-- PERFORMANCE METRICS
-- ==============================================

-- Overall performance statistics
SELECT 
    COUNT(*) as total_transactions,
    ROUND(AVG(execution_time), 2) as avg_execution_time_ms,
    ROUND(MIN(execution_time), 2) as min_execution_time_ms,
    ROUND(MAX(execution_time), 2) as max_execution_time_ms,
    ROUND(STDDEV(execution_time), 2) as stddev_execution_time_ms
FROM transactions
WHERE execution_time > 0;

-- Performance by status
SELECT 
    status,
    COUNT(*) as count,
    ROUND(AVG(execution_time), 2) as avg_time_ms,
    ROUND(MIN(execution_time), 2) as min_time_ms,
    ROUND(MAX(execution_time), 2) as max_time_ms
FROM transactions
WHERE execution_time > 0
GROUP BY status;

-- Percentile analysis (requires SQLite with percentile support)
SELECT 
    ROUND(AVG(CASE WHEN rn <= 0.50 * total THEN execution_time END), 2) as p50_median_ms,
    ROUND(AVG(CASE WHEN rn <= 0.90 * total THEN execution_time END), 2) as p90_ms,
    ROUND(AVG(CASE WHEN rn <= 0.95 * total THEN execution_time END), 2) as p95_ms,
    ROUND(AVG(CASE WHEN rn <= 0.99 * total THEN execution_time END), 2) as p99_ms
FROM (
    SELECT 
        execution_time,
        ROW_NUMBER() OVER (ORDER BY execution_time) as rn,
        COUNT(*) OVER () as total
    FROM transactions
    WHERE execution_time > 0
);

-- ==============================================
-- WALLET ANALYSIS
-- ==============================================

-- Transactions per wallet
SELECT 
    wallet_address,
    COUNT(*) as tx_count,
    ROUND(AVG(execution_time), 2) as avg_time_ms,
    SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as successful,
    SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed
FROM transactions
GROUP BY wallet_address
ORDER BY tx_count DESC;

-- Wallet success rate
SELECT 
    wallet_address,
    COUNT(*) as total_tx,
    SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as successful,
    ROUND(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 2) as success_rate_pct
FROM transactions
GROUP BY wallet_address
ORDER BY success_rate_pct DESC;

-- ==============================================
-- TIME SERIES ANALYSIS
-- ==============================================

-- Transactions per minute
SELECT 
    strftime('%Y-%m-%d %H:%M', submitted_at) as minute,
    COUNT(*) as tx_count,
    ROUND(AVG(execution_time), 2) as avg_time_ms
FROM transactions
GROUP BY minute
ORDER BY minute;

-- Transactions per second (approximate)
SELECT 
    strftime('%Y-%m-%d %H:%M:%S', submitted_at) as second,
    COUNT(*) as tps,
    ROUND(AVG(execution_time), 2) as avg_time_ms
FROM transactions
GROUP BY second
ORDER BY second;

-- Hourly transaction volume
SELECT 
    strftime('%Y-%m-%d %H:00', submitted_at) as hour,
    COUNT(*) as tx_count,
    ROUND(AVG(execution_time), 2) as avg_time_ms,
    COUNT(DISTINCT wallet_address) as unique_wallets
FROM transactions
GROUP BY hour
ORDER BY hour;

-- ==============================================
-- NONCE ANALYSIS
-- ==============================================

-- Nonce distribution per wallet
SELECT 
    wallet_address,
    MIN(nonce) as first_nonce,
    MAX(nonce) as last_nonce,
    MAX(nonce) - MIN(nonce) + 1 as nonce_range,
    COUNT(*) as tx_count
FROM transactions
GROUP BY wallet_address;

-- Find nonce gaps (missing nonces)
SELECT 
    t1.wallet_address,
    t1.nonce + 1 as missing_nonce_start,
    MIN(t2.nonce) - 1 as missing_nonce_end
FROM transactions t1
JOIN transactions t2 
    ON t1.wallet_address = t2.wallet_address 
    AND t2.nonce > t1.nonce
GROUP BY t1.wallet_address, t1.nonce
HAVING MIN(t2.nonce) > t1.nonce + 1
ORDER BY t1.wallet_address, missing_nonce_start;

-- ==============================================
-- ERROR ANALYSIS
-- ==============================================

-- Most common errors
SELECT 
    error,
    COUNT(*) as count,
    GROUP_CONCAT(DISTINCT wallet_address) as affected_wallets
FROM transactions
WHERE error IS NOT NULL AND error != ''
GROUP BY error
ORDER BY count DESC;

-- Failed transactions details
SELECT 
    wallet_address,
    nonce,
    error,
    submitted_at
FROM transactions
WHERE status = 'failed'
ORDER BY submitted_at DESC
LIMIT 20;

-- ==============================================
-- GAS ANALYSIS
-- ==============================================

-- Gas price statistics
SELECT 
    COUNT(*) as tx_count,
    MIN(CAST(gas_price as INTEGER)) as min_gas_price,
    ROUND(AVG(CAST(gas_price as INTEGER)), 0) as avg_gas_price,
    MAX(CAST(gas_price as INTEGER)) as max_gas_price,
    MIN(gas_limit) as min_gas_limit,
    ROUND(AVG(gas_limit), 0) as avg_gas_limit,
    MAX(gas_limit) as max_gas_limit
FROM transactions;

-- Total value transferred (in wei)
SELECT 
    COUNT(*) as tx_count,
    SUM(CAST(value as INTEGER)) as total_value_wei,
    ROUND(SUM(CAST(value as INTEGER)) / 1000000000000000000.0, 6) as total_value_eth
FROM transactions
WHERE status = 'success';

-- ==============================================
-- RECENT ACTIVITY
-- ==============================================

-- Last 10 transactions
SELECT 
    wallet_address,
    tx_hash,
    nonce,
    status,
    ROUND(execution_time, 2) as exec_time_ms,
    submitted_at
FROM transactions
ORDER BY submitted_at DESC
LIMIT 10;

-- Recent successful transactions
SELECT 
    wallet_address,
    tx_hash,
    ROUND(execution_time, 2) as exec_time_ms,
    submitted_at
FROM transactions
WHERE status = 'success'
ORDER BY submitted_at DESC
LIMIT 10;

-- Recent failed transactions
SELECT 
    wallet_address,
    nonce,
    error,
    submitted_at
FROM transactions
WHERE status = 'failed'
ORDER BY submitted_at DESC
LIMIT 10;

-- ==============================================
-- EXPORT QUERIES
-- ==============================================

-- CSV export format (copy result to CSV)
.mode csv
.headers on
.output performance_report.csv
SELECT 
    wallet_address,
    tx_hash,
    nonce,
    status,
    execution_time,
    submitted_at,
    error
FROM transactions
ORDER BY submitted_at;
.output stdout

-- Summary report
SELECT 
    '=== GO-TPS PERFORMANCE REPORT ===' as report,
    '' as value
UNION ALL
SELECT 
    'Total Transactions:',
    CAST(COUNT(*) as TEXT)
FROM transactions
UNION ALL
SELECT 
    'Successful:',
    CAST(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as TEXT)
FROM transactions
UNION ALL
SELECT 
    'Failed:',
    CAST(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as TEXT)
FROM transactions
UNION ALL
SELECT 
    'Avg Execution Time (ms):',
    CAST(ROUND(AVG(execution_time), 2) as TEXT)
FROM transactions
WHERE execution_time > 0
UNION ALL
SELECT 
    'Unique Wallets:',
    CAST(COUNT(DISTINCT wallet_address) as TEXT)
FROM transactions;
