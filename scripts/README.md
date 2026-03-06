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

## Root Wrappers

For convenience, the root directory contains thin wrappers:
```bash
./analyze.sh [command]   # → scripts/analyze.sh
./graph.py               # → scripts/graph_metrics.py
```
