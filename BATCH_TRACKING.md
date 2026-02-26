# Batch Number Feature

## Overview

The batch number feature assigns a unique identifier to each execution of the TPS tester. This allows you to track, compare, and analyze multiple test runs within the same database.

## Batch Number Format

```
batch-YYYYMMDD-HHMMSS
```

**Example:** `batch-20260226-143025`
- Date: February 26, 2026
- Time: 14:30:25

## How It Works

### Single Mode
Each execution gets a unique batch number:
```bash
# First run
./go-tps  # Creates batch-20260226-143025

# Second run (a minute later)
./go-tps  # Creates batch-20260226-143125
```

### Loop Mode
Each iteration within the loop gets its own batch number:
```bash
RUN_DURATION_MINUTES=5 ./go-tps

# Creates multiple batches:
# - batch-20260226-143000 (iteration 1)
# - batch-20260226-143045 (iteration 2)
# - batch-20260226-143130 (iteration 3)
# ... continues until 5 minutes elapsed
```

## Benefits

### 1. **Multiple Tests in One Database**
Run different test configurations and keep all data in one place:
```bash
# Test 1: Low load
WALLET_COUNT=2 TX_PER_WALLET=10 ./go-tps

# Test 2: High load
WALLET_COUNT=20 TX_PER_WALLET=100 ./go-tps

# Both stored in same database, different batch numbers
```

### 2. **Performance Comparison**
Compare results across different runs:
```bash
# List all batches with stats
./analyze.sh batches

# Output shows:
# batch_number              total_tx  success  failed  avg_ms  tps
# batch-20260226-143125     20        20       0       245.5   8.5
# batch-20260226-143025     200       198      2       512.3   6.2
```

### 3. **Loop Mode Tracking**
Track each iteration of a long-running test:
```bash
RUN_DURATION_MINUTES=30 ./go-tps

# After completion, analyze specific iterations:
./analyze.sh batch batch-20260226-143000  # First iteration
./analyze.sh batch batch-20260226-145500  # Last iteration
```

### 4. **Data Isolation**
Query specific test runs without interference:
```sql
-- Get only transactions from a specific run
SELECT * FROM transactions 
WHERE batch_number = 'batch-20260226-143025';

-- Compare TPS across batches
SELECT 
    batch_number,
    COUNT(*) as tx_count,
    ROUND(AVG(execution_time), 2) as avg_ms
FROM transactions
GROUP BY batch_number;
```

## Usage Examples

### View All Batches
```bash
./analyze.sh batches
```

**Output:**
```
=== ALL BATCHES ===
batch_number              total_tx  success  failed  pending  avg_ms  started              completed
batch-20260226-143125     30        30       0       0        245.5   2026-02-26 14:31:25  2026-02-26 14:31:29
batch-20260226-143025     30        28       2       0        512.3   2026-02-26 14:30:25  2026-02-26 14:30:31
```

### View Specific Batch
```bash
./analyze.sh batch batch-20260226-143025
```

**Output:**
```
=== BATCH: batch-20260226-143025 ===

--- Summary ---
Total Transactions: 30
Successful: 28
Failed: 2
Pending: 0
Avg Execution Time: 512.3 ms

--- Time Range ---
Started: 2026-02-26 14:30:25
Completed: 2026-02-26 14:30:31
Duration: 6.0 seconds

--- TPS for this Batch ---
TPS: 5.0 tx/s

--- Wallet Statistics for this Batch ---
wallet          total  success  failed  avg_ms
0x742d97...     10     10       0       498.2
0x8a5c3b...     10     9        1       523.7
0x1f4e2a...     10     9        1       515.8
```

### SQL Queries

**List batches:**
```sql
SELECT DISTINCT batch_number FROM transactions ORDER BY batch_number DESC;
```

**Compare batch performance:**
```sql
SELECT 
    batch_number,
    COUNT(*) as total_tx,
    SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as success,
    ROUND(AVG(execution_time), 2) as avg_ms,
    MIN(submitted_at) as started,
    MAX(submitted_at) as completed
FROM transactions
GROUP BY batch_number
ORDER BY batch_number DESC;
```

**Get best and worst performing batches:**
```sql
-- Best TPS
SELECT 
    batch_number,
    COUNT(*) as tx_count,
    ROUND(CAST(COUNT(*) as REAL) / 
        ((JULIANDAY(MAX(submitted_at)) - JULIANDAY(MIN(submitted_at))) * 86400), 2) as tps
FROM transactions
GROUP BY batch_number
ORDER BY tps DESC
LIMIT 5;

-- Worst average execution time
SELECT 
    batch_number,
    ROUND(AVG(execution_time), 2) as avg_ms
FROM transactions
GROUP BY batch_number
ORDER BY avg_ms DESC
LIMIT 5;
```

## Database Schema Changes

The `transactions` table now includes:
```sql
CREATE TABLE transactions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    batch_number TEXT NOT NULL,           -- NEW
    wallet_address TEXT NOT NULL,
    tx_hash TEXT,
    nonce INTEGER NOT NULL,
    -- ... other fields
);

CREATE INDEX idx_batch_number ON transactions(batch_number);  -- NEW
```

## API Functions

### Database Methods

```go
// Get statistics for a specific batch
func (d *Database) GetBatchStats(batchNumber string) (map[string]interface{}, error)

// List all batch numbers
func (d *Database) ListBatches() ([]string, error)
```

### Usage in Code

```go
// List all batches
batches, err := db.ListBatches()
for _, batch := range batches {
    fmt.Println("Batch:", batch)
}

// Get stats for specific batch
stats, err := db.GetBatchStats("batch-20260226-143025")
fmt.Printf("Total transactions: %v\n", stats["total_transactions"])
fmt.Printf("TPS: %v\n", stats["tps"])
```

## Testing

Test the batch feature:
```bash
# Run the test script
./test_batch_feature.sh

# This will:
# 1. Run two separate executions
# 2. Verify multiple batches exist
# 3. Display batch analysis
```

## Integration with Loop Mode

Loop mode automatically creates a new batch for each iteration:

```bash
RUN_DURATION_MINUTES=10 WALLET_COUNT=5 TX_PER_WALLET=10 ./go-tps
```

**Sample output:**
```
=== Ethereum TPS Tester ===

Running in LOOP MODE for 10 minutes

Loop started at: 14:30:00
Will run until: 14:40:00
============================================================


[ITERATION #1] Time remaining: 10.0 minutes
------------------------------------------------------------
Batch Number: batch-20260226-143000

... execution ...

✓ Iteration complete. Starting next iteration...


[ITERATION #2] Time remaining: 8.5 minutes
------------------------------------------------------------
Batch Number: batch-20260226-143130

... execution ...
```

Each iteration's data can be analyzed independently while maintaining a cumulative database.

## Best Practices

### 1. **Consistent Database Location**
Use the same DB_PATH for related tests:
```bash
export DB_PATH="./performance_tests.db"
./go-tps  # First test
./go-tps  # Second test
./analyze.sh batches  # Compare both
```

### 2. **Naming Convention**
Batch numbers are automatically generated, but you can identify them by time:
- Morning tests: batch-20260226-09xxxx
- Afternoon tests: batch-20260226-14xxxx
- Evening tests: batch-20260226-20xxxx

### 3. **Cleanup**
Remove old batches if needed:
```sql
-- Delete batches older than a specific date
DELETE FROM transactions 
WHERE batch_number LIKE 'batch-202602%';

-- Keep only recent N batches
DELETE FROM transactions 
WHERE batch_number NOT IN (
    SELECT DISTINCT batch_number 
    FROM transactions 
    ORDER BY batch_number DESC 
    LIMIT 10
);
```

### 4. **Export by Batch**
Export specific batch data:
```bash
BATCH="batch-20260226-143025"
sqlite3 transactions.db -header -csv \
  "SELECT * FROM transactions WHERE batch_number = '$BATCH'" \
  > "${BATCH}.csv"
```

## Troubleshooting

**Q: Why do I see the same batch number for multiple transactions?**  
A: All transactions within a single execution share the same batch number. Different executions get different batch numbers.

**Q: Can I customize the batch number format?**  
A: Currently, the format is fixed as `batch-YYYYMMDD-HHMMSS`. To customize, modify the `runSingleExecution()` function in main.go.

**Q: How do I find the most recent batch?**  
A: 
```bash
./analyze.sh batches | head -3
# or
sqlite3 transactions.db "SELECT batch_number FROM transactions ORDER BY id DESC LIMIT 1;"
```

**Q: Can two batches have the same number?**  
A: Extremely unlikely. Batch numbers include seconds in the timestamp, so you'd need to start two executions in the exact same second.
