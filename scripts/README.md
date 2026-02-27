# Scripts Directory

This directory contains analysis and visualization tools for go-tps.

## Files

### analyze.sh
Shell script for easy database analysis and querying.

**Usage:**
```bash
./scripts/analyze.sh [command]
```

**Available commands:**
- `summary` - Overall transaction statistics
- `tps` - TPS metrics (submission & confirmation)
- `performance` - Detailed performance breakdown
- `wallets` - Per-wallet statistics
- `batches` - List all batch executions
- `batch <id>` - Statistics for specific batch
- `recent` - Last 10 transactions
- `errors` - Error analysis
- `timeline` - Time-based analysis
- `export` - Export to CSV
- `query` - Interactive SQL shell

### graph_metrics.py
**Unified Python script for generating both TPS and Latency visualization graphs.**

This is the main graphing tool that combines TPS and latency analysis in one convenient interface.

**Features:**
- Generate TPS graphs, latency graphs, or both
- Saves all images to `images/` folder (organized output)
- Groups data into 1-second intervals
- Displays comprehensive statistics
- Interactive batch selection
- High-quality PNG output (300 DPI)

**Usage:**
```bash
./scripts/graph_metrics.py
```

**What it generates:**

1. **TPS Graph** (`images/tps_graph_*.png`)
   - Blue line: Submission TPS
   - Green line: Confirmation TPS
   - Shows avg/max statistics

2. **Latency Graph** (`images/latency_graph_*.png`)
   - Orange line: Execution Latency (ms) - time to execute RPC submission
   - Purple line: Confirmation Latency (ms) - time from submission to confirmation
   - Shows avg/min/max statistics

3. **Gas Price Graph** (`images/gas_price_graph_*.png`)
   - Blue line: Transaction Gas Price (Gwei) - gas price set when submitting
   - Red line: Effective Gas Price (Gwei) - actual gas price paid from receipt
   - Shows avg/min/max statistics for both prices
   - Useful for analyzing EIP-1559 dynamics

**Interactive Options:**
- Select specific batch or all batches
- Choose TPS, Latency, Gas Price, or combinations
- Press Enter for defaults (most recent batch, all graphs)

**Requirements:**
```bash
pip3 install -r requirements.txt
```

## Backward Compatibility

For convenience, wrapper scripts in the root directory allow you to run:
```bash
./analyze.sh [command]      # Same as ./scripts/analyze.sh
./graph.py                  # Same as ./scripts/graph_metrics.py
```

**Recommended:** Use `./graph.py` for generating all graphs. All output files are saved in the `images/` folder.
