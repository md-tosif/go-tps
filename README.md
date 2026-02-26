# Go TPS (Transactions Per Second) Tester

A Go-based tool for testing Ethereum network transaction throughput. This tool generates multiple wallets from mnemonics, creates batched transactions with precalculated nonces, and tracks performance metrics in a SQLite database.

## Features

- ✅ Generate multiple Ethereum wallets from BIP39 mnemonics
- ✅ Hierarchical Deterministic (HD) wallet support
- ✅ Precalculated nonce management for batch transactions
- ✅ Concurrent transaction submission to a single RPC endpoint
- ✅ SQLite database for transaction tracking and performance analysis
- ✅ Detailed execution time metrics
- ✅ Configurable via environment variables

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

## Output

The tool generates several outputs:

1. **Console Output**: Real-time progress and summary statistics
2. **mnemonic.txt**: Generated mnemonic phrase (KEEP SECURE!)
3. **transactions.db**: SQLite database with all transaction data

### Database Schema

#### Transactions Table
- `id`: Auto-incrementing primary key
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

## Performance Analysis

Query the database for performance metrics:

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
