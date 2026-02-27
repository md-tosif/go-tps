# Go-TPS: Ethereum Transaction Performance Testing Tool

## Quick Overview for AI Assistants

This document provides a comprehensive understanding of the go-tps codebase for AI assistants like Claude. It includes architecture, code organization, key functions, data flow, and common tasks.

---

## Project Purpose

**go-tps** is a Go-based tool designed to test Ethereum network transaction throughput (TPS - Transactions Per Second). It:
- Generates multiple Ethereum wallets from BIP39 mnemonics
- Creates batched transactions with precalculated nonces
- Sends transactions concurrently to an Ethereum RPC endpoint
- Tracks performance metrics in a SQLite database
- Provides analysis tools for transaction performance

**Use Cases:**
- Load testing Ethereum RPC endpoints
- Benchmarking blockchain network performance
- Testing transaction throughput under various conditions
- Analyzing gas costs and execution times

---

## Architecture Overview

### System Components

```
┌─────────────────────────────────────────────────────────────┐
│                     User / Environment                       │
│           (Config via ENV vars or .env file)                 │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│                       main.go                                │
│  • Configuration loading                                     │
│  • Database initialization                                   │
│  • RPC/WebSocket connection management                       │
│  • Orchestration of wallet generation & transaction sending  │
│  • Loop mode for continuous testing                          │
│  • Background receipt confirmation tracking                  │
└──────────────────────┬──────────────────────────────────────┘
                       │
        ┌──────────────┼──────────────┬──────────────┐
        ▼              ▼              ▼              ▼
┌─────────────┐ ┌──────────────┐ ┌─────────────┐ ┌──────────┐
│  wallet.go  │ │transaction.go│ │database.go  │ │ RPC/WS   │
│             │ │              │ │             │ │ Endpoint │
│ • Mnemonic  │ │ • Nonces     │ │ • Schema    │ └──────────┘
│   generation│ │ • Tx create  │ │ • Insert/   │
│ • HD wallet │ │ • Signing    │ │   Update    │
│   derivation│ │ • Sending    │ │ • Queries   │
│ • BIP44     │ │ • Receipts   │ │ • Stats     │
└─────────────┘ └──────────────┘ └─────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│                   SQLite Database                            │
│  • transactions table (batch_number, tx details, metrics)    │
│  • wallets table (addresses, derivation paths)               │
└─────────────────────────────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│                    Analysis Tools                            │
│  • analyze.sh (shell script for common queries)             │
│  • queries.sql (pre-written SQL queries)                    │
└─────────────────────────────────────────────────────────────┘
```

### Key Design Patterns

1. **Batch Execution Model**: Each run/iteration gets a unique batch number (timestamp-based)
2. **Precalculated Nonces**: Fetches starting nonce, then generates sequential nonces for all transactions
3. **Concurrent Wallet Processing**: Uses goroutines to process multiple wallets in parallel
4. **Async Receipt Confirmation**: Receipt waiting happens in background goroutines (non-blocking)
5. **Dual Connection Strategy**: Supports both RPC polling and WebSocket subscriptions for receipts

---

## File Structure & Responsibilities

### Core Go Files

#### `main.go` (537 lines)
**Primary Entry Point**

**Key Functions:**
- `main()` - Application entry point, orchestrates entire flow
- `LoadConfig()` - Loads configuration from environment variables
- `runInLoopMode()` - Continuous execution for specified duration
- `runSingleExecution()` - Single batch execution
- `waitForReceiptInBackground()` - Async receipt confirmation (uses independent DB connection)
- `SaveMnemonicToFile()` - Saves generated mnemonic to file

**Flow:**
1. Load config (with defaults)
2. Initialize database
3. Connect to RPC (and optionally WebSocket)
4. Generate or load mnemonic
5. Derive wallets from mnemonic
6. Display wallet addresses and balances
7. Ask user confirmation
8. Execute transaction sending (single or loop mode)
9. Wait for receipt confirmations (in background)
10. Display summary statistics

**Configuration Variables:**
```go
const (
    DefaultRPCURL             = "http://localhost:8545"
    DefaultWSURL              = "" // Optional WebSocket
    DefaultDBPath             = "./transactions.db"
    DefaultWalletCount        = 10
    DefaultTxPerWallet        = 10
    DefaultValueWei           = "1000000000000000" // 0.001 ETH
    DefaultToAddress          = "0x0000000000000000000000000000000000000001"
    DefaultRunDurationMinutes = 0 // 0 = single run, >0 = loop
)
```

**Important Notes:**
- Uses `sync.WaitGroup` for goroutine synchronization
- Transaction submission returns immediately; receipts confirmed in background
- Loop mode regenerates wallets each iteration unless MNEMONIC is set
- Asks for user confirmation before sending transactions (displays balances)

#### `wallet.go` (125 lines)
**Wallet Management**

**Key Functions:**
- `GenerateMnemonic()` → `(string, error)` - Creates 12-word BIP39 mnemonic
- `DeriveWalletsFromMnemonic(mnemonic string, count int)` → `([]*Wallet, error)`
- `CreateWalletsFromMultipleMnemonics(mnemonicCount, walletsPerMnemonic int)` → `([][]*Wallet, []string, error)`
- `GetPublicAddress(privateKey *ecdsa.PrivateKey)` → `common.Address`
- `ParseDerivationPath(path string)` → `(accounts.DerivationPath, error)`

**Wallet Struct:**
```go
type Wallet struct {
    Address        common.Address
    PrivateKey     *ecdsa.PrivateKey
    DerivationPath string // e.g., "m/44'/60'/0'/0/5"
}
```

**Derivation Path:**
- Uses BIP44 standard: `m/44'/60'/0'/0/i` where i is the wallet index
- 44' = BIP44, 60' = Ethereum coin type
- All wallets derived from single mnemonic for easy management

**Dependencies:**
- `github.com/tyler-smith/go-bip39` - Mnemonic generation
- `github.com/miguelmota/go-ethereum-hdwallet` - HD wallet derivation

#### `transaction.go` (321 lines)
**Transaction Creation, Signing, and Sending**

**Key Structs:**
```go
type TransactionSender struct {
    client  *ethclient.Client
    chainID *big.Int
}

type TxRequest struct {
    Wallet    *Wallet
    ToAddress common.Address
    Value     *big.Int
    Nonce     uint64
    GasPrice  *big.Int
    GasLimit  uint64
}

type TxResult struct {
    TxHash        string
    Nonce         uint64
    Status        string
    SubmittedAt   time.Time
    ExecutionTime float64 // milliseconds
    Error         error
}
```

**Key Functions:**
- `NewTransactionSender(rpcURL string)` → `(*TransactionSender, error)` - Creates RPC client
- `GetNonce(ctx, address)` → `(uint64, error)` - Fetches pending nonce
- `GetGasPrice(ctx)` → `(*big.Int, error)` - Gets suggested gas price
- `GetBalance(ctx, address)` → `(*big.Int, error)` - Gets wallet balance
- `CreateTransaction(req *TxRequest)` → `(*types.Transaction, error)`
- `SignTransaction(tx, wallet)` → `(*types.Transaction, error)` - EIP-155 signing
- `SendTransaction(ctx, signedTx)` → `(*TxResult, error)` - Sends to RPC
- `CreateAndSendTransaction(ctx, req)` → `(*TxResult, error)` - Complete flow
- `PrepareBatchTransactions(ctx, wallet, toAddress, value, count)` → `([]*TxRequest, error)` - **Critical!** Precalculates nonces
- `WaitForReceipt(ctx, txHash, timeout)` → `(*types.Receipt, error)` - RPC polling
- `WaitForReceiptWithSharedWebSocket(ctx, wsClient, txHash, timeout)` → `(*types.Receipt, error)` - WebSocket + RPC fallback

**Receipt Confirmation Strategy:**
```
┌─────────────────────────────────────────────────────────────┐
│ WaitForReceiptWithSharedWebSocket                            │
│                                                              │
│  ┌─────────────────────┐     ┌─────────────────────┐        │
│  │  Goroutine 1:       │     │  Goroutine 2:       │        │
│  │  WebSocket          │     │  RPC Polling        │        │
│  │  Block Headers      │     │  Every 500ms        │        │
│  └──────────┬──────────┘     └──────────┬──────────┘        │
│             │                           │                   │
│             └───────────┬───────────────┘                   │
│                         ▼                                   │
│                  Receipt Channel                            │
│                  (First one wins)                           │
└─────────────────────────────────────────────────────────────┘
```

**Important Notes:**
- Nonce management is critical - uses `PrepareBatchTransactions` to precalculate all nonces
- Uses EIP-155 signature (replay protection)
- Standard gas limit: 21000 (ETH transfer)
- Receipt confirmation runs in parallel (WebSocket subscription + RPC polling)

#### `database.go` (332 lines)
**SQLite Database Operations**

**Key Structs:**
```go
type Transaction struct {
    ID            int64
    BatchNumber   string  // NEW: batch tracking
    WalletAddress string
    TxHash        string
    Nonce         uint64
    ToAddress     string
    Value         string
    GasPrice      string
    GasLimit      uint64
    Status        string  // "pending", "success", "failed"
    SubmittedAt   time.Time
    ConfirmedAt   *time.Time
    ExecutionTime float64  // milliseconds
    Error         string
}

type Database struct {
    db *sql.DB
}
```

**Schema:**
```sql
CREATE TABLE transactions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    batch_number TEXT NOT NULL,
    wallet_address TEXT NOT NULL,
    tx_hash TEXT,
    nonce INTEGER NOT NULL,
    to_address TEXT NOT NULL,
    value TEXT NOT NULL,
    gas_price TEXT NOT NULL,
    gas_limit INTEGER NOT NULL,
    status TEXT NOT NULL,
    submitted_at TIMESTAMP NOT NULL,
    confirmed_at TIMESTAMP,
    execution_time REAL,
    error TEXT
);

CREATE TABLE wallets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    address TEXT NOT NULL UNIQUE,
    derivation_path TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL
);

-- Indexes for performance
CREATE INDEX idx_batch_number ON transactions(batch_number);
CREATE INDEX idx_wallet_address ON transactions(wallet_address);
CREATE INDEX idx_tx_hash ON transactions(tx_hash);
CREATE INDEX idx_status ON transactions(status);
CREATE INDEX idx_submitted_at ON transactions(submitted_at);
```

**Key Functions:**
- `NewDatabase(dbPath string)` → `(*Database, error)` - Opens/creates DB
- `InsertTransaction(tx *Transaction)` → `(int64, error)` - Inserts new transaction
- `UpdateTransactionStatus(txHash, status, confirmedAt, execTime, errMsg)` → `error` - Updates after confirmation
- `InsertWallet(address, derivationPath string)` → `error`
- `GetTransactionStats()` → `(map[string]interface{}, error)` - Overall stats
- `CalculateTPS()` → `(map[string]interface{}, error)` - TPS calculations
- `GetBatchStats(batchNumber string)` → `(map[string]interface{}, error)` - Batch-specific stats
- `ListBatches()` → `([]string, error)` - Lists all batch numbers

**Batch Number Format:**
```
batch-20260226-143025
       │      │
       │      └─ Time (14:30:25)
       └─ Date (2026-02-26)
```

**Key Metrics Calculated:**
- TPS (submission time window)
- TPS (confirmation time window)
- Average execution time
- Success/failure rates
- Per-wallet statistics
- Per-batch comparison

---

## Data Flow

### Single Execution Flow

```
1. User runs: ./go-tps
   ↓
2. Load config from environment variables
   ↓
3. Initialize SQLite database (create tables if needed)
   ↓
4. Connect to Ethereum RPC endpoint
   ↓
5. Generate or load mnemonic phrase
   ↓
6. Derive N wallets from mnemonic (using BIP44)
   ↓
7. Display wallet addresses with balances
   ↓
8. Ask user for confirmation (y/n)
   ↓
9. Generate unique batch number (batch-YYYYMMDD-HHMMSS)
   ↓
10. For each wallet (in parallel goroutines):
    a. Get current nonce from RPC
    b. Precalculate M nonces (nonce, nonce+1, ..., nonce+M-1)
    c. Create and sign M transactions
    d. Send each transaction to RPC
    e. Save to database with status="pending"
    f. Launch background goroutine to wait for receipt
   ↓
11. Wait for all submissions to complete (not receipts)
   ↓
12. Display summary (submission metrics)
   ↓
13. Return to user (receipts still confirming in background)
```

### Loop Mode Flow

```
1. User runs: RUN_DURATION_MINUTES=30 ./go-tps
   ↓
2. Calculate end time = now + 30 minutes
   ↓
3. While current time < end time:
   ↓
   a. Generate new batch number for this iteration
   ↓
   b. Execute single execution flow (steps 10-12 above)
   ↓
   c. Ensure minimum 1 second duration per iteration
   ↓
   d. Display iteration summary and remaining time
   ↓
4. Display total loop summary (iterations, total duration)
```

### Receipt Confirmation Flow (Background)

```
For each transaction:
  ↓
1. Launch independent goroutine
  ↓
2. Create its own DB connection
  ↓
3. Create its own RPC connection
  ↓
4. Use shared WebSocket client (if available)
  ↓
5. Run two parallel strategies:
   - WebSocket: Subscribe to new blocks, check receipt on each block
   - RPC Polling: Check receipt every 500ms
  ↓
6. First strategy to get receipt wins
  ↓
7. Update database:
   - status = "success" or "failed"
   - confirmed_at = timestamp
   - execution_time = total time
  ↓
8. Print confirmation message
  ↓
9. Close connections and exit goroutine
```

---

## Key Features Explained

### 1. Batch Number System

**Purpose:** Track multiple test runs in the same database

**Implementation:**
```go
batchNumber := fmt.Sprintf("batch-%s", time.Now().Format("20060102-150405"))
```

**Benefits:**
- Compare performance across different runs
- Isolate data for specific tests
- Track loop mode iterations separately
- Query by batch for focused analysis

**Usage:**
```bash
# List all batches
./analyze.sh batches

# View specific batch
./analyze.sh batch batch-20260226-143025

# SQL query
SELECT * FROM transactions WHERE batch_number = 'batch-20260226-143025';
```

### 2. Precalculated Nonce Management

**Problem:** In batch transactions, nonce must be sequential. Fetching nonce for each transaction would get the same value.

**Solution:**
```go
func (ts *TransactionSender) PrepareBatchTransactions(...) {
    // Get starting nonce once
    startNonce := GetNonce(wallet.Address)
    
    // Precalculate all nonces
    for i := 0; i < count; i++ {
        nonce := startNonce + uint64(i)
        // Create transaction with this nonce
    }
}
```

**Critical:** This assumes no other process is sending transactions from the same wallet concurrently.

### 3. Parallel Wallet Processing

**Implementation:**
```go
var wg sync.WaitGroup
for _, wallet := range wallets {
    wg.Add(1)
    go func(w *Wallet) {
        defer wg.Done()
        // Send all transactions for this wallet
    }(wallet)
}
wg.Wait()
```

**Benefits:**
- Multiple wallets send transactions concurrently
- Maximizes throughput
- Each wallet maintains sequential nonces

**Constraint:**
- Transactions within a single wallet are sent sequentially (to maintain nonce order)
- Parallel execution is at wallet level, not transaction level

### 4. Async Receipt Confirmation

**Design Decision:** Don't block main program waiting for receipt confirmations

**Worker Pool Pattern (New):**
```go
// Create worker pool with N workers
receiptJobChan := make(chan ReceiptJob, totalTransactions)
startReceiptWorkerPool(workerCount, receiptJobChan, &wg)

// Submit jobs to workers
receiptJobChan <- ReceiptJob{
    DBPath: dbPath,
    RPCURL: rpcURL,
    TxHash: txHash,
    // ... other fields
}

// Workers process jobs concurrently
```

**Benefits:**
- Fixed number of database/RPC connections (one per worker)
- Better resource management than one goroutine per transaction
- Controlled concurrency prevents overwhelming the RPC endpoint
- Workers reuse connections for better performance

**Old Implementation (Deprecated):**
```go
// Background goroutine per transaction
go waitForReceiptInBackground(dbPath, rpcURL, wsClient, txHash, ...)
```

**Why Worker Pool is Better:**
- Old: Creates N*M connections (N wallets * M transactions)
- New: Creates fixed worker connections (configurable, default 10)
- Old: Can overwhelm system with thousands of goroutines
- New: Controlled concurrency with job queue
- Old: Each goroutine creates/closes DB + RPC connections
- New: Workers reuse connections across multiple jobs

**Worker Pool Components:**

```go
// Job structure
type ReceiptJob struct {
    DBPath     string
    RPCURL     string
    WSClient   *ethclient.Client
    TxHash     string
    Nonce      uint64
    StartTime  time.Time
    WalletNum  int
}

// Worker function
func receiptWorker(workerID int, jobChan <-chan ReceiptJob, wg *sync.WaitGroup) {
    // Reuse connections across jobs
    // Process jobs until channel closes
}
```

**Lifecycle:**
```
Main Program                          Worker Pool
     │                                        │
     ├─ Submit Tx1 ────────────────────┐     │
     │                                  └────→ Job Queue → Worker 1
     ├─ Submit Tx2 ────────────────────┐     │             (processes)
     │                                  └────→ Job Queue → Worker 2
     ├─ Submit Tx3 ────────────────────┐     │             (processes)
     │                                  └────→ Job Queue → Worker 3
     │                                         │             (processes)
     ▼                                        ▼
Return to user                        Workers continue processing
(Summary displayed)                   (Update DB when confirmed)
```

### 5. Dual Receipt Strategy

**Why Both WebSocket and RPC Polling?**

**WebSocket (Primary):**
- Faster (real-time block notifications)
- Lower overhead
- Requires WebSocket URL

**RPC Polling (Fallback):**
- Works with any HTTP RPC
- More reliable (WebSocket can disconnect)
- Higher latency (polls every 500ms)

**Implementation:**
```go
func WaitForReceiptWithSharedWebSocket(wsClient, txHash) {
    receiptChan := make(chan *types.Receipt, 1)
    
    // Strategy 1: WebSocket
    go func() {
        for newBlock := range headers {
            receipt := CheckReceipt(txHash)
            if receipt != nil {
                receiptChan <- receipt
            }
        }
    }()
    
    // Strategy 2: RPC Polling
    go func() {
        for range time.Tick(500*Millisecond) {
            receipt := CheckReceipt(txHash)
            if receipt != nil {
                receiptChan <- receipt
            }
        }
    }()
    
    // First one wins
    return <-receiptChan
}
```

### 6. Serialized Database Writes

**Problem:** SQLite locks prevent concurrent writes from multiple goroutines

**Solution:** Single database writer goroutine with channel

```go
// Create database writer channel
dbWriteChan := make(chan *Transaction, totalTransactions)

// Start writer goroutine
go func() {
    for tx := range dbWriteChan {
        db.InsertTransaction(tx)
    }
}()

// Wallet goroutines send to channel (non-blocking)
dbWriteChan <- &Transaction{...}
```

**Benefits:**
- Eliminates "database is locked" errors
- Serializes all initial transaction writes
- Non-blocking for wallet processing goroutines
- Receipt workers still use independent connections for updates

---

## Configuration

### Environment Variables

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `RPC_URL` | string | `http://localhost:8545` | Ethereum RPC endpoint |
| `WS_URL` | string | `""` | Optional WebSocket URL for faster receipts |
| `DB_PATH` | string | `./transactions.db` | SQLite database path |
| `MNEMONIC` | string | `""` | BIP39 mnemonic (empty = generate new) |
| `WALLET_COUNT` | int | `10` | Number of wallets to derive |
| `TX_PER_WALLET` | int | `10` | Transactions per wallet |
| `VALUE_WEI` | string | `"1000000000000000"` | 0.001 ETH in wei |
| `TO_ADDRESS` | string | `0x000...001` | Recipient address |
| `RUN_DURATION_MINUTES` | int | `0` | Loop mode duration (0 = single run) |
| `RECEIPT_WORKERS` | int | `10` | Number of concurrent workers for receipt confirmation |

### Example Configurations

**Local Development:**
```bash
RPC_URL="http://localhost:8545" \
WALLET_COUNT=5 \
TX_PER_WALLET=10 \
./go-tps
```

**High Load Test:**
```bash
RPC_URL="http://localhost:8545" \
WALLET_COUNT=50 \
TX_PER_WALLET=100 \
./go-tps
```

**Loop Mode (Continuous Testing):**
```bash
RUN_DURATION_MINUTES=60 \
RPC_URL="http://localhost:8545" \
WALLET_COUNT=10 \
TX_PER_WALLET=20 \
./go-tps
```

**With WebSocket (Faster Confirmations):**
```bash
RPC_URL="http://localhost:8545" \
WS_URL="ws://localhost:8546" \
WALLET_COUNT=10 \
TX_PER_WALLET=10 \
./go-tps
```

---

## Analysis Tools

### analyze.sh

**Shell script for easy database querying**

**Commands:**
```bash
./analyze.sh summary       # Overall statistics
./analyze.sh tps          # TPS metrics (submission & confirmation)
./analyze.sh performance  # Detailed performance breakdown
./analyze.sh wallets      # Per-wallet statistics
./analyze.sh batches      # List all batches
./analyze.sh batch <id>   # Specific batch details
./analyze.sh recent       # Last 10 transactions
./analyze.sh errors       # Error analysis
./analyze.sh timeline     # Time-based analysis
./analyze.sh export       # Export to CSV
./analyze.sh query        # Interactive SQL shell
```

**Example Output:**
```
$ ./analyze.sh summary
=== TRANSACTION SUMMARY ===
Total Transactions: 300
Successful: 285
Failed: 15
Pending: 0
Unique Wallets: 10
Avg Execution Time: 245.67 ms
```

### queries.sql

**Pre-written SQL queries for common analysis tasks**

**Categories:**
1. Basic Statistics
2. TPS Metrics
3. Performance Analysis
4. Wallet Analysis
5. Time Series Analysis
6. Nonce Analysis
7. Error Analysis
8. Gas Analysis
9. Batch Analysis
10. Export Queries

**Example Query:**
```sql
-- Compare TPS across batches
SELECT 
    batch_number,
    COUNT(*) as tx_count,
    ROUND(CAST(COUNT(*) as REAL) / 
        ((JULIANDAY(MAX(submitted_at)) - JULIANDAY(MIN(submitted_at))) * 86400), 2) as tps
FROM transactions
GROUP BY batch_number
ORDER BY tps DESC;
```

---

## Common Tasks (For AI Assistants)

### Task 1: Add New Configuration Option

**Steps:**
1. Add constant in main.go:
```go
const DefaultNewOption = "value"
```

2. Add to Config struct:
```go
type Config struct {
    // ...
    NewOption string
}
```

3. Load in LoadConfig():
```go
NewOption: getEnv("NEW_OPTION", DefaultNewOption),
```

4. Use in code where needed

5. Document in README.md

### Task 2: Modify Transaction Parameters

**Location:** `transaction.go`, `PrepareBatchTransactions()`

**Example: Dynamic Gas Limit**
```go
// Current: Fixed 21000
GasLimit: 21000,

// Change to: Dynamic based on contract call
GasLimit: estimateGas(toAddress, data),
```

### Task 3: Add New Database Query

**Option 1: Add to analyze.sh**
```bash
new_command() {
    echo "=== NEW QUERY ==="
    sqlite3 "$DB_PATH" -header -column <<EOF
SELECT field1, field2 FROM transactions WHERE condition;
EOF
}

case "${1:-help}" in
    # ...
    new-command)
        new_command
        ;;
esac
```

**Option 2: Add to queries.sql**
```sql
-- New analysis query
SELECT ...
FROM transactions
WHERE ...;
```

### Task 4: Modify Batch Number Format

**Location:** `main.go`, `runSingleExecution()`

**Current:**
```go
batchNumber := fmt.Sprintf("batch-%s", time.Now().Format("20060102-150405"))
```

**Example Change:**
```go
// Add custom prefix
batchNumber := fmt.Sprintf("test-%s-%s", customPrefix, time.Now().Format("20060102-150405"))
```

### Task 5: Add Custom Transaction Data

**Steps:**
1. Modify `TxRequest` in transaction.go:
```go
type TxRequest struct {
    // ...existing fields...
    Data []byte  // Add this
}
```

2. Modify transaction creation:
```go
func (ts *TransactionSender) CreateTransaction(req *TxRequest) (*types.Transaction, error) {
    tx := types.NewTransaction(
        req.Nonce,
        req.ToAddress,
        req.Value,
        req.GasLimit,
        req.GasPrice,
        req.Data,  // Use custom data
    )
    return tx, nil
}
```

3. Update `PrepareBatchTransactions` to populate Data field

---

## Dependencies

### Direct Dependencies

```go
require (
    github.com/ethereum/go-ethereum v1.17.0
    github.com/mattn/go-sqlite3 v1.14.34
    github.com/miguelmota/go-ethereum-hdwallet v0.1.3
    github.com/tyler-smith/go-bip39 v1.1.0
    github.com/joho/godotenv v1.5.1
)
```

**go-ethereum:** Ethereum protocol implementation
- Transaction creation and signing
- RPC client
- Types (Address, Hash, Transaction, Receipt)
- Cryptographic functions

**go-sqlite3:** SQLite driver (CGo required)
- Database operations
- Pure Go not possible (uses C library)

**go-ethereum-hdwallet:** HD wallet derivation
- BIP44 derivation path
- Wallet generation from mnemonic

**go-bip39:** BIP39 mnemonic implementation
- Mnemonic generation
- Entropy creation

**godotenv:** Environment variable loading
- .env file support
- Optional (doesn't fail if missing)

---

## Security Considerations

### Sensitive Data

**mnemonic.txt:**
- Contains recovery phrase for all wallets
- Can regenerate all private keys
- **NEVER commit to version control**
- **NEVER share publicly**

**Private Keys:**
- Generated from mnemonic
- Only exist in memory during execution
- Not saved to disk (except in mnemonic form)

**Database:**
- Contains wallet addresses
- Contains transaction hashes
- Does NOT contain private keys
- Safe to share for analysis (but may reveal test patterns)

### Best Practices

1. **Use Test Networks:** Never use mainnet for testing
2. **Limit Funds:** Only fund wallets with amounts you're willing to lose
3. **Secure Mnemonic Storage:** Keep mnemonic.txt in secure location
4. **Environment Variables:** Don't hardcode sensitive data
5. **Git Ignore:** Ensure .gitignore covers sensitive files

---

## Performance Characteristics

### Bottlenecks

1. **RPC Endpoint:** Primary bottleneck
   - Sequential transaction submission per wallet
   - Receipt polling latency
   - Network round-trip time

2. **Database Writes:** Mitigated bottleneck
   - SQLite is single-writer
   - Solution: Serialized database writer goroutine
   - All initial inserts go through single channel
   - Receipt workers have independent connections

3. **Nonce Management:** Design constraint
   - Must maintain sequential nonces per wallet
   - Cannot parallelize within single wallet

### Optimization Opportunities

1. **Increase Wallet Count:**
   - More parallelism at wallet level
   - Linear scaling up to RPC limits

2. **Use WebSocket:**
   - Faster receipt confirmation
   - Lower latency than polling

3. **Batch RPC Calls:**
   - Not currently implemented
   - Could batch multiple SendTransaction calls
   - Requires JSON-RPC batch support

4. **Serialized Database Writes:**
   - Implemented using database writer goroutine
   - Prevents SQLite "database is locked" errors
   - All initial transaction inserts go through single channel

---

## Testing Strategies

### Unit Testing

**Current State:** No unit tests
**Recommended:** Add tests for:
- Wallet generation
- Nonce calculation
- Transaction creation
- Database operations

### Integration Testing

**Manual Testing:**
```bash
# 1. Start local Ethereum node (Hardhat/Ganache)
npx hardhat node

# 2. Run with minimal config
RPC_URL="http://localhost:8545" WALLET_COUNT=2 TX_PER_WALLET=2 ./go-tps

# 3. Verify in database
sqlite3 transactions.db "SELECT * FROM transactions;"
```

### Load Testing

**Gradual Increase:**
```bash
# Start small
WALLET_COUNT=1 TX_PER_WALLET=1 ./go-tps

# Increase gradually
WALLET_COUNT=5 TX_PER_WALLET=5 ./go-tps
WALLET_COUNT=10 TX_PER_WALLET=10 ./go-tps
WALLET_COUNT=20 TX_PER_WALLET=20 ./go-tps
```

---

## Troubleshooting Guide

### Common Errors

**1. "insufficient funds for gas * price + value"**
- **Cause:** Wallet has no ETH
- **Solution:** Fund wallets before sending transactions
- **Prevention:** Check balances (tool does this automatically)

**2. "nonce too low"**
- **Cause:** Nonce already used
- **Solution:** Use fresh wallets or generate new mnemonic
- **Prevention:** Don't run multiple instances with same wallets

**3. "connection refused"**
- **Cause:** RPC endpoint not running
- **Solution:** Start Ethereum node or check RPC_URL
- **Prevention:** Verify endpoint before running

**4. "database is locked"**
- **Cause:** Multiple processes accessing database OR legacy concurrency issue
- **Solution:** Fixed in current version with serialized database writer
- **Old versions:** Close other connections or use one process at a time
- **Current:** Should not occur - serialized writes prevent locking

**5. "replacement transaction underpriced"**
- **Cause:** Transaction with same nonce already pending
- **Solution:** Wait for previous transaction or increase gas price
- **Prevention:** Ensure sequential execution

### Debug Mode

**Enable Verbose Logging:**
```bash
# Currently not implemented
# Could add: LOG_LEVEL=debug ./go-tps
```

**Database Inspection:**
```bash
# Check pending transactions
sqlite3 transactions.db "SELECT COUNT(*) FROM transactions WHERE status='pending';"

# Check last batch
sqlite3 transactions.db "SELECT batch_number FROM transactions ORDER BY id DESC LIMIT 1;"

# Check for errors
sqlite3 transactions.db "SELECT error, COUNT(*) FROM transactions WHERE error IS NOT NULL GROUP BY error;"
```

---

## Future Enhancement Ideas

### Feature Suggestions

1. **EIP-1559 Support:** Add maxFeePerGas and maxPriorityFeePerGas
2. **Contract Interaction:** Support contract calls with data
3. **Token Transfers:** ERC-20 token support
4. **Metrics Export:** Prometheus/Grafana integration
5. **Web Dashboard:** Real-time monitoring UI
6. **Dynamic Gas Estimation:** Use eth_estimateGas
7. **Retry Logic:** Automatic retry for failed transactions
8. **Rate Limiting:** Respect RPC rate limits
9. **Multiple Recipients:** Randomize to addresses
10. **Gas Price Oracle:** Use external oracle for gas price

### Code Quality Improvements

1. **Unit Tests:** Add comprehensive test coverage
2. **Error Handling:** More granular error types
3. **Logging:** Structured logging (e.g., logrus, zap)
4. **Configuration Validation:** Validate config before execution
5. **Graceful Shutdown:** Handle SIGINT/SIGTERM
6. **Progress Bar:** Visual progress indicator
7. **Health Checks:** RPC endpoint health check before execution

---

## API Reference

### Main Types

```go
// Configuration
type Config struct {
    RPCURL             string
    WSURL              string
    DBPath             string
    Mnemonic           string
    WalletCount        int
    TxPerWallet        int
    ValueWei           string
    ToAddress          string
    RunDurationMinutes int
}

// Wallet
type Wallet struct {
    Address        common.Address
    PrivateKey     *ecdsa.PrivateKey
    DerivationPath string
}

// Transaction Request
type TxRequest struct {
    Wallet    *Wallet
    ToAddress common.Address
    Value     *big.Int
    Nonce     uint64
    GasPrice  *big.Int
    GasLimit  uint64
}

// Transaction Result
type TxResult struct {
    TxHash        string
    Nonce         uint64
    Status        string
    SubmittedAt   time.Time
    ExecutionTime float64
    Error         error
}

// Database Transaction
type Transaction struct {
    ID            int64
    BatchNumber   string
    WalletAddress string
    TxHash        string
    Nonce         uint64
    ToAddress     string
    Value         string
    GasPrice      string
    GasLimit      uint64
    Status        string
    SubmittedAt   time.Time
    ConfirmedAt   *time.Time
    ExecutionTime float64
    Error         string
}
```

### Key Function Signatures

```go
// Wallet Management
func GenerateMnemonic() (string, error)
func DeriveWalletsFromMnemonic(mnemonic string, count int) ([]*Wallet, error)

// Transaction Sender
func NewTransactionSender(rpcURL string) (*TransactionSender, error)
func (ts *TransactionSender) GetNonce(ctx context.Context, address common.Address) (uint64, error)
func (ts *TransactionSender) PrepareBatchTransactions(ctx context.Context, wallet *Wallet, toAddress common.Address, value *big.Int, count int) ([]*TxRequest, error)
func (ts *TransactionSender) CreateAndSendTransaction(ctx context.Context, req *TxRequest) (*TxResult, error)
func (ts *TransactionSender) WaitForReceiptWithSharedWebSocket(ctx context.Context, wsClient *ethclient.Client, txHash common.Hash, timeout time.Duration) (*types.Receipt, error)

// Database Operations
func NewDatabase(dbPath string) (*Database, error)
func (d *Database) InsertTransaction(tx *Transaction) (int64, error)
func (d *Database) UpdateTransactionStatus(txHash, status string, confirmedAt *time.Time, executionTime float64, errMsg string) error
func (d *Database) GetTransactionStats() (map[string]interface{}, error)
func (d *Database) GetBatchStats(batchNumber string) (map[string]interface{}, error)
func (d *Database) ListBatches() ([]string, error)
```

---

## Glossary

**TPS:** Transactions Per Second - measure of throughput
**Nonce:** Transaction sequence number per account
**BIP39:** Bitcoin Improvement Proposal 39 - mnemonic standard
**BIP44:** Bitcoin Improvement Proposal 44 - HD wallet derivation
**HD Wallet:** Hierarchical Deterministic wallet
**Mnemonic:** 12-word recovery phrase
**Derivation Path:** Path to derive specific key from master key
**EIP-155:** Ethereum Improvement Proposal 155 - replay protection
**Gas:** Computation cost on Ethereum
**Gas Price:** Price per unit of gas (in wei)
**Gas Limit:** Maximum gas allowed for transaction
**Wei:** Smallest unit of ETH (1 ETH = 10^18 wei)
**Receipt:** Transaction confirmation from blockchain
**RPC:** Remote Procedure Call - HTTP API for Ethereum
**WebSocket:** Persistent connection for real-time updates
**Batch Number:** Unique identifier for test execution

---

## Quick Reference Commands

```bash
# Build
go build -o go-tps .
make build

# Run (basic)
./go-tps

# Run (custom config)
RPC_URL="http://localhost:8545" WALLET_COUNT=10 TX_PER_WALLET=10 ./go-tps

# Run (loop mode)
RUN_DURATION_MINUTES=30 ./go-tps

# Analysis
./analyze.sh summary
./analyze.sh tps
./analyze.sh batches
./analyze.sh batch batch-20260226-143025

# Database
sqlite3 transactions.db
sqlite3 transactions.db "SELECT * FROM transactions LIMIT 10;"
sqlite3 transactions.db ".schema"

# Export
./analyze.sh export

# Clean
make clean
rm -f go-tps transactions.db mnemonic.txt
```

---

## File Checklist

**Source Code:**
- [x] main.go - Main application
- [x] wallet.go - Wallet management
- [x] transaction.go - Transaction handling
- [x] database.go - Database operations

**Build & Config:**
- [x] go.mod - Dependencies
- [x] go.sum - Dependency checksums
- [x] Makefile - Build automation

**Documentation:**
- [x] README.md - Comprehensive guide
- [x] QUICKSTART.md - Quick start guide
- [x] PROJECT_STRUCTURE.md - File structure
- [x] BATCH_TRACKING.md - Batch feature docs
- [x] claude.md - This file

**Tools:**
- [x] analyze.sh - Analysis script
- [x] queries.sql - SQL queries

**Generated:**
- [ ] go-tps - Compiled binary
- [ ] transactions.db - Database (runtime)
- [ ] mnemonic.txt - Generated mnemonics (runtime)

---

## Version Information

- **Go Version:** 1.25.2
- **go-ethereum:** v1.17.0
- **Current Features:** Batch tracking, loop mode, async receipt confirmation, WebSocket support
- **Last Updated:** February 2026

---

## Contact & Contributing

This project is for testing and educational purposes. Contributions welcome:
- Bug fixes
- Feature enhancements
- Documentation improvements
- Test coverage

**Important:** Always test on test networks. Never use mainnet for development.

---

*End of claude.md*
