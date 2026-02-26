# Go-TPS Project Structure

## Overview
This project is a comprehensive Ethereum transaction performance testing tool written in Go. It generates wallets from mnemonics, creates and sends multiple transactions with precalculated nonces, and stores performance metrics in a SQLite database.

## Project Files

### Core Application Files

#### `main.go` (7.1 KB)
The main entry point of the application. Contains:
- Configuration loading from environment variables
- Orchestration of wallet generation
- Transaction batch creation and sending
- Performance metrics collection and reporting
- Database operations coordination

**Key Features:**
- Configurable via environment variables
- Generates and saves mnemonics
- Manages the complete transaction lifecycle
- Provides real-time progress updates
- Calculates and displays performance statistics

#### `wallet.go` (3.0 KB)
Handles all wallet-related operations:
- BIP39 mnemonic generation (12-word phrases)
- HD wallet derivation using BIP44 standard (m/44'/60'/0'/0/x)
- Multiple wallet generation from mnemonics
- Private key and address management

**Key Functions:**
- `GenerateMnemonic()` - Creates new BIP39 mnemonic
- `DeriveWalletsFromMnemonic()` - Derives multiple wallets from one mnemonic
- `CreateWalletsFromMultipleMnemonics()` - Generates wallets from multiple mnemonics

#### `transaction.go` (4.5 KB)
Manages Ethereum transaction creation and sending:
- Transaction creation with custom parameters
- Transaction signing using EIP-155
- Nonce management and precalculation
- RPC connection management
- Batch transaction preparation

**Key Components:**
- `TransactionSender` - Main struct for handling transactions
- `PrepareBatchTransactions()` - Precalculates nonces for batch sending
- `CreateAndSendTransaction()` - Complete transaction lifecycle
- `SendMultipleTransactions()` - Batch transaction sender

#### `database.go` (4.2 KB)
SQLite database operations for performance tracking:
- Schema creation and management
- Transaction record storage
- Wallet information storage
- Performance statistics queries

**Database Tables:**
1. `transactions` - Stores all transaction details and metrics
2. `wallets` - Stores wallet addresses and derivation paths

**Key Features:**
- Automatic schema initialization
- Indexed queries for performance
- Transaction status tracking
- Execution time recording

### Configuration Files

#### `.env.example` (716 B)
Template environment configuration file with all available options:
- RPC_URL - Ethereum RPC endpoint
- DB_PATH - SQLite database location
- MNEMONIC_COUNT - Number of mnemonics to generate
- WALLETS_PER_MNEMONIC - Wallets per mnemonic
- TX_PER_WALLET - Transactions per wallet
- VALUE_WEI - Transaction value in wei
- TO_ADDRESS - Recipient address

#### `.gitignore` (446 B)
Git ignore rules to prevent committing:
- Compiled binaries
- Database files
- Sensitive data (mnemonics, keys)
- IDE configuration files
- Log files

### Build and Automation Files

#### `Makefile` (1.8 KB)
Build automation and common commands:
- `make build` - Build the project
- `make run` - Build and run with defaults
- `make run-local` - Run with local RPC
- `make clean` - Remove build artifacts and database
- `make test` - Run tests
- `make fmt` - Format code
- `make install` - Install/update dependencies
- `make build-all` - Cross-platform builds

#### `go.mod` (1.9 KB)
Go module definition with dependencies:
- github.com/ethereum/go-ethereum - Ethereum client library
- github.com/mattn/go-sqlite3 - SQLite driver
- github.com/tyler-smith/go-bip39 - BIP39 mnemonic implementation
- github.com/miguelmota/go-ethereum-hdwallet - HD wallet support

#### `go.sum` (28 KB)
Dependency checksums for reproducible builds

### Documentation Files

#### `README.md` (6.6 KB)
Comprehensive project documentation:
- Feature overview
- Installation instructions
- Configuration options
- Usage examples
- Security warnings
- Troubleshooting guide
- Development guidelines

#### `QUICKSTART.md` (4.9 KB)
Step-by-step guide for quick setup:
- Prerequisites
- Build instructions
- Wallet funding guide
- Configuration examples
- Common workflows
- Troubleshooting tips

#### `queries.sql` (7.2 KB)
Pre-written SQL queries for performance analysis:
- Basic statistics
- Performance metrics
- Wallet analysis
- Time series analysis
- Nonce tracking
- Error analysis
- Gas analysis
- Export queries

### Analysis Tools

#### `analyze.sh` (5.5 KB) - Executable
Shell script for easy database analysis:
- `./analyze.sh summary` - Overall statistics
- `./analyze.sh performance` - Performance metrics
- `./analyze.sh wallets` - Per-wallet statistics
- `./analyze.sh recent` - Recent transactions
- `./analyze.sh errors` - Error analysis
- `./analyze.sh timeline` - Time-based analysis
- `./analyze.sh export` - Export to CSV
- `./analyze.sh query` - Interactive SQL shell

### Binary

#### `go-tps` (17 MB) - Executable
Compiled binary (Linux x86-64):
- Statically linked with CGo for SQLite support
- Contains all dependencies
- Ready to run on compatible Linux systems

## Generated Files (Not in Repository)

### `transactions.db`
SQLite database created at runtime:
- Stores all transaction records
- Wallet information
- Performance metrics
- Can be analyzed with SQLite tools or analyze.sh

### `mnemonics.txt`
Generated mnemonic phrases (KEEP SECURE!):
- Contains all generated mnemonics
- Required to recover wallets
- Never commit to version control
- Automatically ignored by .gitignore

## Workflow

```
┌─────────────────────────────────────────────────────────────┐
│                         User Input                           │
│              (Environment Variables/Defaults)                │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                       main.go                                │
│  • Load configuration                                        │
│  • Initialize database                                       │
│  • Connect to RPC                                            │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                      wallet.go                               │
│  • Generate mnemonics (BIP39)                                │
│  • Derive wallets (BIP44: m/44'/60'/0'/0/x)                  │
│  • Create private keys                                       │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                    transaction.go                            │
│  • Get starting nonce for each wallet                        │
│  • Precalculate nonces for all transactions                  │
│  • Create transactions                                       │
│  • Sign transactions (EIP-155)                               │
│  • Send to RPC endpoint                                      │
│  • Track execution time                                      │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                     database.go                              │
│  • Store transaction details                                 │
│  • Record wallet information                                 │
│  • Save execution times                                      │
│  • Track status (pending/success/failed)                     │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                       Output                                 │
│  • Console statistics                                        │
│  • transactions.db                                           │
│  • mnemonics.txt                                             │
└─────────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                  analyze.sh / queries.sql                    │
│  • Query database                                            │
│  • Generate reports                                          │
│  • Export data                                               │
└─────────────────────────────────────────────────────────────┘
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

---

For questions or issues, refer to the main README.md or open an issue.
