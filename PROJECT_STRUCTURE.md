# Go-TPS Project Structure

## Overview

go-tps is an Ethereum transaction throughput benchmarking tool written in Go. It derives multiple wallets from a BIP39 mnemonic, submits batched transactions with pre-calculated nonces in parallel, and stores performance metrics in a SQLite database.

## Documentation

| File | Purpose |
|------|---------|
| [README.md](README.md) | Full feature reference, configuration, usage examples |
| [QUICKSTART.md](QUICKSTART.md) | Step-by-step getting-started guide |
| [PROJECT_STRUCTURE.md](PROJECT_STRUCTURE.md) | This file |
| [queries.sql](queries.sql) | Pre-written SQL analysis queries |
| [scripts/README.md](scripts/README.md) | Analysis and graphing script reference |
| [claude.md](claude.md) | Deep technical reference for developers/AI |

---

## Source Files

### `main.go`
Application entry point. Responsibilities:
- Load config from environment variables (with `.env` file support)
- Initialise four per-level log files in `logs/`
- Connect to RPC and optionally WebSocket
- Derive wallets, display balances, prompt for confirmation
- Launch a **DB writer pool** to serialise SQLite inserts (preventing lock contention) and dispatch receipt jobs only _after_ each INSERT succeeds
- Launch a **receipt worker pool** of long-lived goroutines that each reuse one RPC connection; workers retry timed-out receipts up to 3 times before marking a transaction failed
- Process all wallets in parallel (`sync.WaitGroup`), pre-calculate nonces locally, and queue `DBWriteJob`s
- Loop mode: repeat execution for a configurable duration with a unique batch number per iteration

**Key types / functions:**
- `Config` struct + `LoadConfig()` — all configuration
- `ReceiptJob` / `DBWriteJob` — worker job types
- `startDBWriterPool()` / `dbWriterWorker()` — serialised DB inserts
- `startReceiptWorkerPool()` / `receiptWorker()` / `processReceiptJob()` — async receipt confirmation
- `runSingleExecution()` / `runInLoopMode()` — execution modes
- `logDebug/logInfo/logWarn/logError()` — log to both console and per-level files

### `wallet.go`
Wallet derivation and key management.
- `GenerateMnemonic()` — creates a 128-bit BIP39 mnemonic (12 words)
- `DeriveWalletsFromMnemonic(mnemonic, count, txSender)` — derives wallets via BIP44 (`m/44'/60'/0'/0/i`); when `txSender` is non-nil, each wallet's current pending nonce is pre-fetched from the RPC during derivation
- `CreateWalletsFromMultipleMnemonics()` — utility for generating wallets from several mnemonics
- `Wallet` struct — holds `Address`, `PrivateKey`, `DerivationPath`, and pre-fetched `Nonce`

### `transaction.go`
Transaction creation, signing, sending, and receipt waiting.
- `TransactionSender` — wraps an `ethclient.Client` + chain ID
- `PrepareBatchTransactions()` — builds `TxRequest` slice using locally-incremented nonces from `wallet.Nonce`; fetches gas price once per batch
- `CreateAndSendTransaction()` — creates, signs (EIP-155), and sends a transaction; returns `TxResult` with submission time and latency
- `WaitForReceipt()` — RPC polling fallback
- `WaitForReceiptWithSharedWebSocket()` — runs WebSocket block subscription and RPC polling in parallel; first result wins

### `database.go`
SQLite persistence layer.
- `Database` struct with a `sync.Mutex` protecting all write operations
- `InsertTransaction()` — stores a new record with status `pending`
- `UpdateTransactionStatus()` — sets `status`, `confirmed_at`, `gas_used`, `effective_gas_price` after confirmation
- `GetTransactionStats()` / `GetBatchStats()` — summary and per-batch statistics
- `CalculateTPS()` — computes submission-window and confirmation-window TPS
- `GetFailedTransactions()` — retrieves recent failures for the post-run summary
- Schema: `transactions` table (with indexes on `batch_number`, `wallet_address`, `tx_hash`, `status`, `submitted_at`) + `wallets` table

---

## Configuration

### `.env.example`
Template for all environment variables. Copy to `.env` and edit:
```
RPC_URL=http://localhost:8545
WS_URL=
DB_PATH=./transactions.db
MNEMONIC=
WALLET_COUNT=10
TX_PER_WALLET=10
VALUE_WEI=1000000000000000
TO_ADDRESS=0x0000000000000000000000000000000000000001
RUN_DURATION_MINUTES=0
RECEIPT_WORKERS=10
LOG_LEVEL=DEBUG
```

### `.gitignore`
Excludes: compiled binary, `*.db` files, `mnemonic.txt`, `.env`, `logs/`, `images/`.

---

## Build and Automation

### `Makefile`
| Target | Action |
|--------|--------|
| `make build` | Compile `go-tps` binary |
| `make run` | Build and run with defaults |
| `make run-local` | Build and run against `http://localhost:8545` |
| `make clean` | Remove binary, `*.db`, `mnemonic.txt` |
| `make test` | `go test ./...` |
| `make fmt` | `go fmt ./...` |
| `make install` | `go mod download && go mod tidy` |
| `make build-all` | Cross-compile for Linux, macOS, Windows |

### `go.mod` / `go.sum`
Module definition (`go 1.25.2`) and checksums. Direct dependencies:
- `github.com/ethereum/go-ethereum v1.17.0`
- `github.com/mattn/go-sqlite3 v1.14.34` (requires CGo)
- `github.com/miguelmota/go-ethereum-hdwallet v0.1.3`
- `github.com/tyler-smith/go-bip39 v1.1.0`
- `github.com/joho/godotenv v1.5.1`

---

## Documentation

| File | Contents |
|------|----------|
| `README.md` | Full feature reference, config table, schema, how-it-works, troubleshooting |
| `QUICKSTART.md` | Step-by-step setup guide for new users |
| `PROJECT_STRUCTURE.md` | This file |
| `queries.sql` | ~10 categories of pre-written SQL analysis queries |
| `claude.md` | Architecture diagrams, API reference, data-flow, task guide for developers/AI |

---

## Analysis Tools

### `scripts/analyze.sh` (also `./analyze.sh` wrapper)
SQLite analysis shell script.

| Command | Output |
|---------|--------|
| `summary` | Total, success, failure, avg latency |
| `tps` | Submission and confirmation TPS |
| `performance` | Execution time breakdown |
| `wallets` | Per-wallet transaction counts and latency |
| `batches` | All batch numbers |
| `batch <id>` | Stats for a specific batch |
| `recent` | Last 10 transactions |
| `errors` | Error message breakdown |
| `timeline` | Time-series transaction counts |
| `export` | Dump transactions to CSV |
| `query` | Interactive `sqlite3` shell |

### `scripts/graph_metrics.py` (also `./graph.py` wrapper)
Python graphing tool. Generates PNG graphs in `images/`.
- **TPS graph** — submission TPS (blue) + confirmation TPS (green)
- **Latency graph** — RPC submission latency (orange) + confirmation latency (purple)
- **Gas price graph** — signed gas price vs effective gas price from receipt

Requires: `pip3 install -r requirements.txt`

### `scripts/get-gas.py`
Helper script for analysing gas price data from the database.

---

## Runtime-Generated Files (not in repository)

| File/Dir | Contents |
|----------|----------|
| `transactions.db` | SQLite database — all transaction records and wallet info |
| `mnemonic.txt` | Generated BIP39 mnemonic — **keep secure, never commit** |
| `logs/debug.log` | Appended debug-level log (created on first run) |
| `logs/info.log` | Appended info-level log |
| `logs/warn.log` | Appended warning-level log |
| `logs/error.log` | Appended error-level log |
| `images/*.png` | Graph outputs from `graph.py` |
| `go-tps` | Compiled binary |

## Data Flow

```
env vars / .env
      │
      ▼
    main.go ───────────── wallet.go (derive + prefetch nonces)
      │                           │
      │     receipt worker pool ◄─┘
      │     db writer pool     ◄─── transaction.go (create/sign/send)
      │                                   │
      ▼                                   ▼
  database.go (insert pending)    database.go (update confirmed)
      │
      ▼
  transactions.db
      │
      ▼
  analyze.sh / graph.py / queries.sql
```

## Dependencies

### Direct Dependencies
- **go-ethereum** (v1.17.0) - Ethereum protocol implementation
- **go-sqlite3** (v1.14.34) - SQLite driver for Go (requires CGo)
- **go-bip39** (v1.1.0) - BIP39 mnemonic implementation
- **go-ethereum-hdwallet** (v0.1.3) - HD wallet derivation

### Transitive Dependencies
- Various cryptography, networking, and utility libraries
- See go.mod and go.sum for complete dependency tree

## Security Considerations

### Sensitive Files (Never Commit!)
- `mnemonics.txt` - Can recover all wallets
- `*.db` - May contain sensitive transaction data
- `.env` - May contain API keys or private configuration

### Safe to Commit
- Source code files (*.go)
- Documentation (*.md)
- Configuration templates (.env.example)
- Build files (Makefile, go.mod)
- Analysis tools (analyze.sh, queries.sql)

## Performance Characteristics

### Memory Usage
- Approximately 20-50 MB base
- Scales with number of concurrent transactions
- SQLite adds minimal overhead

### Disk Usage
- Binary: ~17 MB
- Database: Scales with transaction count (~1 KB per transaction)
- Minimal temporary file usage

### Network I/O
- One HTTP connection to RPC endpoint
- Sequential transaction sending
- Configurable batch size via TX_PER_WALLET

## Extending the Project

### Adding New Features
1. **Custom Transaction Types**: Modify transaction.go
2. **Additional Database Queries**: Add to queries.sql
3. **New Analysis Tools**: Enhance analyze.sh
4. **Alternative Wallet Sources**: Extend wallet.go
5. **Parallel Transaction Sending**: Modify main.go orchestration

### Testing Different Networks
- Local: Hardhat, Ganache, Anvil
- Testnet: Sepolia, Goerli, Mumbai
- Mainnet: (Use with extreme caution!)

## File Size Summary
```
Total project size: ~17.5 MB (including binary)
Code files only: ~30 KB
Documentation: ~20 KB
Binary: ~17 MB
Dependencies metadata: ~30 KB
```

## Next Steps

1. Read [QUICKSTART.md](QUICKSTART.md) for usage instructions
2. Configure your RPC endpoint
3. Fund generated wallets
4. Run the tool
5. Analyze results with analyze.sh or queries.sql

## Documentation Summary

### For Users
- **Start here**: [QUICKSTART.md](QUICKSTART.md) — Get running in 10 minutes
- **Reference**: [README.md](README.md) — Complete feature documentation
- **Analysis**: `./analyze.sh` — Quick database queries

### For Developers
- **Technical Details**: [claude.md](claude.md) — Architecture and code deep-dive
- **Project Layout**: [PROJECT_STRUCTURE.md](PROJECT_STRUCTURE.md) — This file
- **Database Queries**: [queries.sql](queries.sql) — SQL examples

---

For questions or issues, refer to the appropriate documentation file or open an issue on GitHub.
