# Go TPS (Transactions Per Second) Tester

A Go-based tool for testing Ethereum network transaction throughput. This tool generates multiple wallets from mnemonics, creates batched transactions with precalculated nonces, and tracks performance metrics in a SQLite database.

## Features

- ✅ Generate multiple Ethereum wallets from BIP39 mnemonics
- ✅ Hierarchical Deterministic (HD) wallet support
- ✅ Precalculated nonce management for batch transactions
- ✅ Concurrent transaction submission to a single RPC endpoint
- ✅ Parallel wallet processing with goroutines
- ✅ Async receipt waiting (non-blocking confirmations)
- ✅ Loop mode for continuous testing over time
- ✅ SQLite database for transaction tracking and performance analysis
- ✅ Detailed execution time metrics and TPS calculations
- ✅ Configurable via environment variables or .env file

## Prerequisites

- Go 1.19 or higher
- Access to an Ethereum RPC endpoint (local or remote)
- Sufficient ETH in generated wallets to pay for gas fees

## Installation

```bash
# Clone or navigate to the project directory
cd go-tps

# Install dependencies
go mod download

# Build the project
go build -o go-tps
```

## Configuration

Configure the application using environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `RPC_URL` | Ethereum RPC endpoint URL | `http://localhost:8545` |
| `DB_PATH` | SQLite database file path | `./transactions.db` |
| `MNEMONIC` | BIP39 mnemonic phrase (leave empty to auto-generate) | `` (empty - generates new) |
| `WALLET_COUNT` | Number of wallets to derive from mnemonic | `10` |
| `TX_PER_WALLET` | Number of transactions per wallet | `10` |
| `VALUE_WEI` | Transaction value in wei | `1000000000000000` (0.001 ETH) |
| `TO_ADDRESS` | Recipient address for all transactions | `0x0000000000000000000000000000000000000001` |
| `RUN_DURATION_MINUTES` | Duration to run in loop mode (0 = single run) | `0` |

## Usage

### Basic Usage

Run with default configuration:

```bash
./go-tps
```

### Custom Configuration

```bash
# Using environment variables
export RPC_URL="https://mainnet.infura.io/v3/YOUR_PROJECT_ID"
export WALLET_COUNT=15
export TX_PER_WALLET=20
export TO_ADDRESS="0xYourRecipientAddress"

./go-tps
```

Or use inline environment variables:

```bash
RPC_URL="http://localhost:8545" \
WALLET_COUNT=10 \
TX_PER_WALLET=15 \
./go-tps
```

### Using a Specific Mnemonic

If you want to reuse an existing mnemonic:

```bash
MNEMONIC="word1 word2 word3 word4 word5 word6 word7 word8 word9 word10 word11 word12" \
RPC_URL="http://localhost:8545" \
WALLET_COUNT=10 \
./go-tps
```

### Example for Local Development

If you're running a local Ethereum node (e.g., Hardhat, Ganache, or Geth):

```bash
# Start your local node first, then:
RPC_URL="http://localhost:8545" \
WALLET_COUNT=5 \
TX_PER_WALLET=5 \
./go-tps
```

### Wallet Funding Check

Before starting transactions, the tool automatically displays all wallet addresses with their current balances and asks for confirmation.

**Example output:**
```
============================================================
WALLET ADDRESSES AND BALANCES
============================================================

[1] 0x742d97eE84D7324bf022038B27f97a01000E39F1
    Balance: 5000000000000000000 wei (5.000000 ETH)

[2] 0x8a5c3bF4f1C80E2D9a4B5e6d7F8c9a1b2e3f4a5b
    Balance: 0 wei (0.000000 ETH)
    ⚠️  WARNING: Wallet has ZERO balance!

[3] 0x1f4e2a3b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f
    Balance: 1000000000000000000 wei (1.000000 ETH)

============================================================
⚠️  WARNING: Some wallets have zero balance or errors!

Do you want to proceed with sending transactions? (y/n):
```

**Features:**
- Shows wallet addresses with balances in both wei and ETH
- Warns if any wallet has zero balance
- Requires user confirmation (y/yes) before proceeding
- Press 'n' or any other key to cancel and exit

**Tips:**
- Fund all wallets before running the tool
- Ensure sufficient gas fees for transactions
- Use test networks for initial testing

### Loop Mode (Continuous Testing)

By default, the tool runs once and exits. You can enable **Loop Mode** to continuously run the testing process for a specified duration using the `RUN_DURATION_MINUTES` environment variable.

**When to use Loop Mode:**
- Load testing over extended periods
- Stress testing RPC endpoints
- Continuous performance monitoring
- Long-duration TPS benchmarking

**Example: Run for 5 minutes**
```bash
RUN_DURATION_MINUTES=5 \
RPC_URL="http://localhost:8545" \
WALLET_COUNT=3 \
TX_PER_WALLET=10 \
./go-tps
```

**Example: Run for 30 minutes with higher load**
```bash
RUN_DURATION_MINUTES=30 \
RPC_URL="http://localhost:8545" \
WALLET_COUNT=10 \
TX_PER_WALLET=20 \
./go-tps
```

**Loop Mode Behavior:**
- Runs continuously until the specified time duration elapses
- Each iteration generates new wallets and transactions
- All iterations share the same database file (cumulative data)
- Shows iteration count and remaining time
- 2-second delay between iterations

**Note:** In loop mode, the mnemonic will be regenerated for each iteration unless you specify `MNEMONIC` environment variable to reuse the same wallets.

## Output

The tool generates several outputs:

1. **Console Output**: Real-time progress and summary statistics
2. **mnemonic.txt**: Generated mnemonic phrase (KEEP SECURE!)
3. **transactions.db**: SQLite database with all transaction data

### Database Schema

#### Transactions Table
- `id`: Auto-incrementing primary key
- `batch_number`: Unique identifier for each execution run
- `wallet_address`: Sender wallet address
- `tx_hash`: Transaction hash
- `nonce`: Transaction nonce
- `to_address`: Recipient address
- `value`: Transaction value in wei
- `gas_price`: Gas price in wei
- `gas_limit`: Gas limit
- `status`: Transaction status (pending/success/failed)
- `submitted_at`: Submission timestamp
- `confirmed_at`: Confirmation timestamp
- `execution_time`: Time to submit in milliseconds
- `error`: Error message if failed

#### Wallets Table
- `id`: Auto-incrementing primary key
- `address`: Wallet address
- `derivation_path`: HD wallet derivation path
- `created_at`: Creation timestamp

### Batch Tracking

Each execution (single run or loop iteration) is assigned a unique batch number in the format `batch-YYYYMMDD-HHMMSS`. This allows you to:

- Track multiple test runs in the same database
- Compare performance across different executions
- Analyze specific test iterations in loop mode
- Export data for individual test runs

**Example batch numbers:**
- `batch-20260226-143025` - Single run at 14:30:25 on Feb 26, 2026
- `batch-20260226-143510` - Loop iteration at 14:35:10

**Query by batch:**
```bash
# List all batches
./analyze.sh batches

# View specific batch statistics
./analyze.sh batch batch-20260226-143025

# SQL query for specific batch
sqlite3 transactions.db "SELECT * FROM transactions WHERE batch_number = 'batch-20260226-143025';"
```

## Performance Analysis

The tool includes a comprehensive analysis script with batch support:

```bash
# View all batches
./analyze.sh batches

# View specific batch details
./analyze.sh batch batch-20260226-143025

# Other analysis commands
./analyze.sh summary       # Overall summary
./analyze.sh tps          # TPS metrics
./analyze.sh performance  # Detailed performance
./analyze.sh wallets      # Per-wallet stats
./analyze.sh timeline     # Timeline analysis
```

Query the database directly for custom analysis:

```bash
# Install sqlite3 if not already installed
sudo apt-get install sqlite3  # Ubuntu/Debian
brew install sqlite3           # macOS

# Query the database
sqlite3 transactions.db

# Example queries:
sqlite> SELECT COUNT(*) as total_tx FROM transactions;
sqlite> SELECT AVG(execution_time) as avg_time_ms FROM transactions WHERE execution_time > 0;
sqlite> SELECT status, COUNT(*) FROM transactions GROUP BY status;
sqlite> SELECT wallet_address, COUNT(*) as tx_count FROM transactions GROUP BY wallet_address;
```

## Project Structure

```
go-tps/
├── main.go           # Main application entry point
├── wallet.go         # Wallet generation and mnemonic handling
├── transaction.go    # Transaction creation and sending
├── database.go       # SQLite database operations
├── go.mod            # Go module dependencies
├── go.sum            # Dependency checksums
├── README.md         # This file
└── .gitignore        # Git ignore rules
```

## Important Security Notes

⚠️ **WARNING**: The generated `mnemonic.txt` file contains sensitive information that can be used to access the wallets and any funds they contain. 

- **Never commit mnemonic.txt to version control**
- **Store mnemonics securely**
- **Use test networks for experimentation**
- **Fund wallets only with amounts you're willing to lose during testing**

## How It Works

1. **Mnemonic Management**: Uses provided mnemonic or generates a new one using BIP39 standard
2. **Wallet Generation**: Derives multiple wallets from single mnemonic using BIP44 (m/44'/60'/0'/0/x)
3. **Nonce Management**: Fetches the current nonce for each wallet and precalculates nonces for all transactions
4. **Transaction Creation**: Creates and signs multiple transactions for each wallet
5. **Batch Submission**: Sends transactions to the RPC endpoint sequentially, tracking execution time
6. **Data Storage**: Saves all transaction details and performance metrics to SQLite database

## Troubleshooting

### Connection Issues
```
Error connecting to RPC: dial tcp: lookup localhost: no such host
```
Solution: Ensure your RPC endpoint is running and accessible.

### Insufficient Funds
Transactions will fail if wallets don't have enough ETH. Fund the generated wallets before running the test.

### Database Locked
```
Error: database is locked
```
Solution: Close any other programs accessing the database file.

## Development

### Run Tests
```bash
go test ./...
```

### Format Code
```bash
go fmt ./...
```

### Build for Different Platforms
```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o go-tps-linux

# macOS
GOOS=darwin GOARCH=amd64 go build -o go-tps-macos

# Windows
GOOS=windows GOARCH=amd64 go build -o go-tps.exe
```

## Contributing

Feel free to open issues or submit pull requests for improvements.

## License

MIT License - See LICENSE file for details

## Dependencies

- `github.com/ethereum/go-ethereum` - Ethereum Go client
- `github.com/mattn/go-sqlite3` - SQLite driver
- `github.com/tyler-smith/go-bip39` - BIP39 mnemonic implementation
- `github.com/miguelmota/go-ethereum-hdwallet` - HD wallet implementation

## Disclaimer

This tool is for testing and educational purposes only. Use at your own risk. The authors are not responsible for any loss of funds or other damages.
