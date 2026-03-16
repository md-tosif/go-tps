# Scripts Directory

Analysis and visualisation tools for go-tps.

## Files

### `analyze.sh`
Shell script for querying the SQLite database. Also available as `./analyze.sh` in the project root (wrapper).

```bash
./scripts/analyze.sh [command]
```

| Command | Output |
|---------|--------|
| `summary` | Total, success, failure, avg latency |
| `tps` | Submission and confirmation TPS |
| `performance` | Execution time breakdown |
| `wallets` | Per-wallet transaction counts and latency |
| `batches` | List all batch executions |
| `batch <id>` | Stats for a specific batch |
| `recent` | Last 10 transactions |
| `errors` | Error message breakdown |
| `timeline` | Time-series transaction counts |
| `export` | Dump transactions to CSV |
| `query` | Interactive `sqlite3` shell |

### `graph_metrics.py`
Unified Python graphing tool. Also available as `./graph.py` in the project root (wrapper). Saves all images to `images/`.

**Requirements:**
```bash
pip3 install -r requirements.txt
```

**Usage:**
```bash
./scripts/graph_metrics.py
```

Interactive prompts let you select a batch and graph type:

| Graph | File | Description |
|-------|------|-------------|
| TPS | `images/tps_graph_<batch>.png` | Submission TPS (blue) + confirmation TPS (green) |
| Latency | `images/latency_graph_<batch>.png` | RPC submission latency (orange) + confirmation latency (purple) |
| Gas Price | `images/gas_price_graph_<batch>.png` | Signed gas price vs effective gas price from receipt |

All graphs group data into 1-second intervals and display avg/min/max statistics. Output is high-quality PNG (300 DPI).

### `get-gas.py`
Helper script for analysing gas price data from the database. Useful for examining gas price trends across batches.

### `export-address-transactions.js`
Node.js script to export all transactions from an Ethereum address to CSV format using the Etherscan API.

**Requirements:**
- Node.js 14+ 
- Etherscan API key (get free key at [etherscan.io/apis](https://etherscan.io/apis))

**Features:**
- Exports all normal transactions for any Ethereum address
- Optional internal transactions support
- Comprehensive transaction details: hash, block, timestamp, gas data, fees, status
- Automatic pagination (handles addresses with 10,000+ transactions)
- Rate limiting (respects Etherscan API limits)
- CSV format with proper escaping

**Usage:**
```bash
# Basic export (normal transactions only)
ETHERSCAN_API_KEY=your_key node export-address-transactions.js 0x742d35cc6460c0dbc25b35b5c65d5ebaeacadc21

# Include internal transactions
ETHERSCAN_API_KEY=your_key node export-address-transactions.js --include-internal 0x742d35cc6460c0dbc25b35b5c65d5ebaeacadc21

# Custom output directory
ETHERSCAN_API_KEY=your_key OUTPUT_DIR=./exports node export-address-transactions.js 0x742d35cc6460c0dbc25b35b5c65d5ebaeacadc21

# Help
node export-address-transactions.js --help
```

**Output:**
- CSV file: `<address>_<type>_transactions_<timestamp>.csv`
- Headers: hash, blockNumber, timeStamp, from, to, value, gas, gasPrice, gasUsed, txnFee, status, isError, input, contractAddress, cumulativeGasUsed, confirmations
- Transaction summary with totals and date range

### `export-example.sh`
Interactive example script that demonstrates how to use the address transaction exporter with well-known Ethereum addresses or custom addresses.

**Requirements:**
- Etherscan API key set as `ETHERSCAN_API_KEY` environment variable

**Usage:**
```bash
export ETHERSCAN_API_KEY=your_key_here
./scripts/export-example.sh
```

Features interactive selection of example addresses and export options.

## Root Wrappers

For convenience, the root directory contains thin wrappers:
```bash
./analyze.sh [command]   # → scripts/analyze.sh
./graph.py               # → scripts/graph_metrics.py
```
