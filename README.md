# Go TPS (Transactions Per Second) Tester

A Go-based tool for testing Ethereum network transaction throughput. This tool generates multiple wallets from mnemonics, creates batched transactions with precalculated nonces, and tracks performance metrics in a SQLite database.

## 📚 Documentation

- **[QUICKSTART.md](QUICKSTART.md)** - Quick start guide for first-time users
- **[PROJECT_STRUCTURE.md](PROJECT_STRUCTURE.md)** - Detailed project structure and file descriptions
- **[BATCH_TRACKING.md](BATCH_TRACKING.md)** - Guide to batch tracking feature
- **[claude.md](claude.md)** - Comprehensive technical documentation for AI assistants
- **[queries.sql](queries.sql)** - Pre-written SQL queries for analysis
- **[scripts/analyze.sh](scripts/analyze.sh)** - Shell script for easy database analysis
- **[scripts/graph_metrics.py](scripts/graph_metrics.py)** - Unified TPS and latency visualization tool

## 📑 Table of Contents

- [Features](#features)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Configuration](#configuration)
- [Usage](#usage)
  - [Basic Usage](#basic-usage)
  - [Custom Configuration](#custom-configuration)
  - [Using a Specific Mnemonic](#using-a-specific-mnemonic)
  - [Wallet Funding Check](#wallet-funding-check)
  - [Loop Mode](#loop-mode-continuous-testing)
  - [Log Levels](#log-levels)
- [Output](#output)
- [Performance Analysis](#performance-analysis)
- [Project Structure](#project-structure)
- [Important Security Notes](#important-security-notes)
- [How It Works](#how-it-works)
- [Troubleshooting](#troubleshooting)
- [Development](#development)
- [Contributing](#contributing)
- [Dependencies](#dependencies)
- [Disclaimer](#disclaimer)

## Features

### Core Features

- ✅ **Multi-Wallet Generation** - Generate multiple Ethereum wallets from BIP39 mnemonics
- ✅ **HD Wallet Support** - Hierarchical Deterministic (HD) wallet support using BIP44
- ✅ **Smart Nonce Management** - Precalculated nonce management for batch transactions
- ✅ **Concurrent Execution** - Parallel wallet processing with goroutines
- ✅ **Batch Tracking** - Unique batch numbers for tracking multiple test runs
- ✅ **Async Receipt Confirmation** - Non-blocking receipt waiting with WebSocket + RPC polling
- ✅ **Loop Mode** - Continuous testing over specified time duration
- ✅ **Performance Metrics** - SQLite database for transaction tracking and TPS calculations
- ✅ **Detailed Analysis** - Built-in analysis tools (analyze.sh) and pre-written SQL queries
- ✅ **Flexible Configuration** - Environment variables or .env file configuration

### Technical Features

- 🔐 **Secure Key Management** - BIP39 mnemonic generation and secure key derivation
- 🚀 **High Performance** - Optimized for maximum throughput testing
- 📊 **Rich Analytics** - TPS calculations, execution times, success rates, and more
- 🔄 **Dual Receipt Strategy** - WebSocket subscriptions + RPC polling for faster confirmations
- 💾 **Persistent Storage** - SQLite database with indexed queries for fast analysis
- 🎯 **User-Friendly** - Balance checks, confirmation prompts, and real-time progress updates

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
| `WS_URL` | WebSocket URL for faster receipt confirmations (optional) | `` (empty) |
| `DB_PATH` | SQLite database file path | `./transactions.db` |
| `MNEMONIC` | BIP39 mnemonic phrase (leave empty to auto-generate) | `` (empty - generates new) |
| `WALLET_COUNT` | Number of wallets to derive from mnemonic | `10` |
| `TX_PER_WALLET` | Number of transactions per wallet | `10` |
| `VALUE_WEI` | Transaction value in wei | `1000000000000000` (0.001 ETH) |
| `TO_ADDRESS` | Recipient address for all transactions | `0x0000000000000000000000000000000000000001` |
| `RUN_DURATION_MINUTES` | Duration to run in loop mode (0 = single run) | `0` |
| `RECEIPT_WORKERS` | Number of concurrent workers for receipt confirmation | `10` |
| `LOG_LEVEL` | Log level: DEBUG, INFO, WARN, ERROR | `INFO` |

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

### Log Levels

Control console output verbosity with the `LOG_LEVEL` environment variable. This helps you focus on the information you need and reduce noise.

**Available Log Levels:**

- **DEBUG**: Shows all logs including detailed transaction info, wallet addresses, balances, nonce details
  - Use for: Debugging issues, understanding detailed flow, development
  
- **INFO** (default): Shows major milestones and progress updates
  - Use for: Normal operations, monitoring progress, production runs
  
- **WARN**: Shows only warnings and errors
  - Use for: Production monitoring, when you only care about issues
  
- **ERROR**: Shows only critical errors
  - Use for: Silent operation where only failures matter

**Examples:**

```bash
# Show all detailed debug information
LOG_LEVEL=DEBUG ./go-tps

# Show only major milestones (default)
LOG_LEVEL=INFO ./go-tps

# Show only warnings and errors
LOG_LEVEL=WARN ./go-tps

# Show only errors (silent mode)
LOG_LEVEL=ERROR ./go-tps
```

**What each level shows:**

| Level | Initializing | Wallet Details | Transaction Submission | Receipt Confirmations | Errors |
|-------|-------------|----------------|------------------------|----------------------|--------|
| DEBUG | ✓ | ✓ | ✓ | ✓ | ✓ |
| INFO  | ✓ | ✗ | Summary only | Success only | ✓ |
| WARN  | ✗ | ✗ | ✗ | Failures only | ✓ |
| ERROR | ✗ | ✗ | ✗ | ✗ | ✓ |

**Note:** Summary reports, headers, and user prompts are always displayed regardless of log level.

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
./scripts/analyze.sh batches

# View specific batch details
./scripts/analyze.sh batch batch-20260226-143025

# Other analysis commands
./scripts/analyze.sh summary       # Overall summary
./scripts/analyze.sh tps          # TPS metrics
./scripts/analyze.sh performance  # Detailed performance
./scripts/analyze.sh wallets      # Per-wallet stats
./scripts/analyze.sh timeline     # Timeline analysis
```

### Performance Graphs

Visualize transaction performance metrics with the unified graphing tool:

**Installation:**
```bash
# Install Python dependencies
pip3 install -r requirements.txt
```

**Quick Start:**
```bash
# Generate both TPS and Latency graphs
./graph.py

# Or use the full path
./scripts/graph_metrics.py
```

**Available Graph Types:**

1. **TPS Graph** - Transactions Per Second
   - Shows throughput over time
   - Blue line: Submission TPS (sent to RPC)
   - Green line: Confirmation TPS (mined in blocks)
   - Displays avg/max statistics

2. **Latency Graph** - Transaction Timing
   - Shows latency in milliseconds over time
   - Orange line: Execution Latency (time to execute RPC call)
   - Purple line: Confirmation Latency (time to mine)
   - Displays avg/min/max statistics

**Features:**
- Groups data into 1-second intervals for clear visualization
- Interactive batch selection (recent or historical)
- Choose specific graph type or generate both
- All graphs saved in `images/` folder
- High-quality PNG output (300 DPI)

**Example Usage:**
```bash
$ ./graph.py
=== Transaction Metrics Graph Generator ===

Available batches:
  0. All batches (combined)
  1. batch-20260226-143025
  2. batch-20260226-142510

Select batch number (0 for all, or press Enter for most recent): 1

Selected batch: batch-20260226-143025

Select graph type:
  1. TPS Graph (Transactions Per Second)
  2. Latency Graph (Transaction Timing)
  3. Both Graphs

Enter choice (1-3, or press Enter for both): 

Generating both graphs...

--- TPS Graph ---
Calculating TPS intervals...
Generating graph...
✓ TPS graph saved to: images/tps_graph_batch-20260226-143025.png

--- Latency Graph ---
Calculating latency intervals...
Generating graph...
✓ Latency graph saved to: images/latency_graph_batch-20260226-143025.png

Done! All graphs saved in the 'images/' directory.
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
├── main.go              # Main application entry point
├── wallet.go            # Wallet generation and mnemonic handling
├── transaction.go       # Transaction creation and sending
├── database.go          # SQLite database operations
├── requirements.txt     # Python dependencies for analysis tools
├── queries.sql          # Pre-written SQL queries
├── scripts/             # Analysis and visualization tools
│   ├── README.md        # Scripts documentation
│   ├── analyze.sh       # Shell script for database analysis
│   └── graph_metrics.py # Unified TPS & Latency graphing tool
├── analyze.sh           # Wrapper for scripts/analyze.sh
├── graph.py             # Wrapper for scripts/graph_metrics.py
├── images/              # Output folder for all generated graphs
│   ├── tps_graph_*.png        # TPS graphs
│   └── latency_graph_*.png    # Latency graphs
├── go.mod               # Go module dependencies
├── go.sum               # Dependency checksums
├── README.md            # This file
├── QUICKSTART.md        # Quick start guide
├── PROJECT_STRUCTURE.md # Project structure documentation
├── BATCH_TRACKING.md    # Batch tracking guide
├── claude.md            # Technical documentation
└── .gitignore           # Git ignore rules
```

**Note:** All graph output files are saved in the `images/` folder for better organization. Use `./graph.py` for generating TPS and latency graphs.

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

Contributions are welcome! Here's how you can help:

1. **Bug Reports** - Open an issue with detailed reproduction steps
2. **Feature Requests** - Suggest improvements or new features
3. **Code Contributions** - Submit pull requests with tests
4. **Documentation** - Improve or translate documentation
5. **Testing** - Test on different networks and report results

### Development Workflow

```bash
# Fork and clone the repository
git clone https://github.com/yourusername/go-tps.git
cd go-tps

# Create a feature branch
git checkout -b feature/your-feature

# Make changes and test
go build -o go-tps .
./go-tps

# Run tests (when available)
go test ./...

# Format code
go fmt ./...

# Commit and push
git add .
git commit -m "Add your feature"
git push origin feature/your-feature

# Open a pull request
```

### Code Style

- Follow standard Go conventions
- Use `go fmt` for formatting
- Add comments for complex logic
- Write descriptive commit messages
- Include tests for new features

## License

MIT License - See LICENSE file for details

## Dependencies

- `github.com/ethereum/go-ethereum` - Ethereum Go client
- `github.com/mattn/go-sqlite3` - SQLite driver
- `github.com/tyler-smith/go-bip39` - BIP39 mnemonic implementation
- `github.com/miguelmota/go-ethereum-hdwallet` - HD wallet implementation

## Disclaimer

This tool is for testing and educational purposes only. Use at your own risk. The authors are not responsible for any loss of funds or other damages.

**Important Warnings:**
- ⚠️ Never use on mainnet unless you fully understand the risks
- ⚠️ Always test with small amounts first
- ⚠️ Keep your mnemonics secure
- ⚠️ Monitor gas costs during testing
- ⚠️ Be aware of RPC rate limits

## Advanced Usage

### WebSocket Support for Faster Confirmations

Enable WebSocket for real-time receipt confirmations:

```bash
RPC_URL="http://localhost:8545" \
WS_URL="ws://localhost:8546" \
./go-tps
```

This enables dual-strategy receipt confirmation:
- WebSocket subscriptions (real-time, faster)
- RPC polling (fallback, more reliable)

### Batch Comparison

Compare performance across multiple test runs:

```bash
# Run multiple tests with different configurations
WALLET_COUNT=5 TX_PER_WALLET=10 ./go-tps
WALLET_COUNT=10 TX_PER_WALLET=10 ./go-tps
WALLET_COUNT=20 TX_PER_WALLET=10 ./go-tps

# Compare all batches
./analyze.sh batches
```

### Custom Analysis

Write custom SQL queries for specific analysis:

```bash
sqlite3 transactions.db
```

```sql
-- Find slowest transactions
SELECT tx_hash, execution_time 
FROM transactions 
WHERE status = 'success' 
ORDER BY execution_time DESC 
LIMIT 10;

-- Calculate success rate by wallet
SELECT 
    wallet_address,
    ROUND(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 2) as success_rate
FROM transactions
GROUP BY wallet_address;

-- Hourly transaction volume
SELECT 
    strftime('%H:00', submitted_at) as hour,
    COUNT(*) as tx_count,
    ROUND(AVG(execution_time), 2) as avg_time_ms
FROM transactions
GROUP BY hour;
```

### Tips & Best Practices

**1. Wallet Management**
- Reuse mnemonics for consistent testing
- Keep track of which wallets are funded
- Use separate mnemonics for different test scenarios

**2. Performance Optimization**
- Increase `WALLET_COUNT` for more parallelism
- Use WebSocket for faster receipt confirmations
- Monitor RPC endpoint performance
- Test during off-peak hours for consistent results

**3. Database Management**
- Regularly backup your database for long-term data
- Use batch numbers to organize different test runs
- Export data periodically for external analysis
- Clean up old test data if disk space is limited

**4. Network Testing**
- Start with small loads (2-3 wallets, 2-3 tx each)
- Gradually increase load to find limits
- Monitor RPC endpoint for errors or rate limiting
- Use local nodes for maximum control

**5. Analysis & Reporting**
- Run `./analyze.sh summary` after each test
- Compare TPS across different configurations
- Export data to CSV for spreadsheet analysis
- Track trends over time with loop mode

### Environment File (.env)

Create a `.env` file for persistent configuration:

```bash
# .env file
RPC_URL=http://localhost:8545
WS_URL=ws://localhost:8546
DB_PATH=./transactions.db
WALLET_COUNT=10
TX_PER_WALLET=10
VALUE_WEI=1000000000000000
TO_ADDRESS=0x0000000000000000000000000000000000000001
RUN_DURATION_MINUTES=0
```

Then simply run:
```bash
./go-tps
```

### Integration with CI/CD

Example GitHub Actions workflow:

```yaml
name: TPS Test

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - uses: actions/setup-go@v2
        with:
          go-version: '1.19'
      
      - name: Build
        run: go build -o go-tps .
      
      - name: Start local node
        run: |
          npx hardhat node &
          sleep 5
      
      - name: Run TPS test
        run: |
          RPC_URL="http://localhost:8545" \
          WALLET_COUNT=2 \
          TX_PER_WALLET=2 \
          ./go-tps
      
      - name: Analyze results
        run: ./analyze.sh summary
```

## FAQ

**Q: Can I use this on mainnet?**
A: Technically yes, but it's **strongly discouraged**. Use test networks for safety.

**Q: How many wallets should I use?**
A: Start with 5-10 wallets. Increase based on your RPC endpoint's capacity.

**Q: Why are my transactions failing?**
A: Common reasons: insufficient funds, nonce conflicts, gas price too low, RPC issues.

**Q: Can I stop the tool while it's running?**
A: Yes, but some transactions may be pending. Receipt confirmations will be incomplete.

**Q: How do I delete old test data?**
A: Delete specific batches with SQL or remove `transactions.db` entirely.

**Q: What's the maximum TPS I can achieve?**
A: Depends on: RPC endpoint capacity, network conditions, wallet count, hardware.

**Q: Can I test ERC-20 token transfers?**
A: Not currently. This version only supports ETH transfers. ERC-20 support is a potential future enhancement.

**Q: How long does receipt confirmation take?**
A: Typically 10-30 seconds, varies with network congestion and block time.

## Changelog

### Version 1.0 (Current)
- Initial release
- Batch tracking feature
- Loop mode support
- WebSocket + RPC dual-strategy receipts
- Async receipt confirmation
- Comprehensive analysis tools

## Roadmap

Potential future enhancements:
- [ ] EIP-1559 transaction support (Type 2)
- [ ] ERC-20 token transfer testing
- [ ] Contract interaction support
- [ ] Dynamic gas price adjustment
- [ ] Web-based dashboard
- [ ] Prometheus metrics export
- [ ] Multi-RPC endpoint support
- [ ] Automatic retry logic
- [ ] Real-time TPS monitoring
- [ ] Custom transaction data

## Support

For questions, issues, or feature requests:
- Open an issue on GitHub
- Review existing documentation
- Check the troubleshooting section
- Consult [claude.md](claude.md) for technical details

---

**Happy Testing! 🚀**
