# Quick Start Guide

## Prerequisites
- Go 1.19+ installed
- An Ethereum RPC endpoint (local testnet recommended)
- Funded wallets (the tool generates new wallets that need ETH)

## Step 1: Build the Project
```bash
# From the project directory
go build -o go-tps .

# Or using make
make build
```

## Step 2: Fund Your Wallets (IMPORTANT!)

The tool generates wallets from a mnemonic. By default, it will generate a new mnemonic and derive 10 wallets from it. To send transactions, these wallets need ETH for gas fees.

### Option A: Use a Local Testnet (Recommended for Testing)

1. Start a local Ethereum node (e.g., Hardhat or Ganache):
```bash
# Example with Hardhat
npx hardhat node
```

2. Note the default funded accounts from your local node

3. Send ETH to the generated wallets (addresses will be shown when you run the tool)

### Option B: Use a Public Testnet

1. Get testnet ETH from a faucet (Sepolia, Goerli, etc.)
2. Run the tool once to generate wallets and see their addresses
3. Fund those addresses from your funded test account
4. Run the tool again to send transactions

## Step 3: Configure (Optional)

Copy the example environment file:
```bash
cp .env.example .env
# Edit .env with your settings
```

Or use environment variables directly:
```bash
export RPC_URL="http://localhost:8545"
export WALLET_COUNT=10
export TX_PER_WALLET=10
```

### Using an Existing Mnemonic

If you want to reuse a specific mnemonic (e.g., from a previous run):
```bash
export MNEMONIC="word1 word2 word3 word4 word5 word6 word7 word8 word9 word10 word11 word12"
export WALLET_COUNT=10
```

## Step 4: Run the Tool

### First Run (Generate Wallets)
```bash
./go-tps
```

This will:
- Generate a new mnemonic (or use provided one via MNEMONIC env var)
- Derive 10 wallets from that mnemonic
- Save mnemonic to `mnemonic.txt`
- Create database at `transactions.db`
- Attempt to send transactions (will fail if wallets have no ETH)

### After Funding Wallets

Once you've funded the generated wallet addresses with ETH, you can reuse the same mnemonic:

```bash
# Use the mnemonic from mnemonic.txt file
MNEMONIC="$(cat mnemonic.txt | tail -n 1)" ./go-tps

# Or set it manually
MNEMONIC="your twelve word mnemonic phrase goes here like this example phrase" ./go-tps
```

## Step 5: Analyze Results

Check the console output for real-time statistics, or query the database:

```bash
sqlite3 transactions.db
```

Example queries:
```sql
-- View all transactions
SELECT * FROM transactions LIMIT 10;

-- Get performance stats
SELECT 
    COUNT(*) as total,
    AVG(execution_time) as avg_time_ms,
    MIN(execution_time) as min_time_ms,
    MAX(execution_time) as max_time_ms
FROM transactions 
WHERE execution_time > 0;

-- Group by status
SELECT status, COUNT(*) as count 
FROM transactions 
GROUP BY status;

-- View wallet transaction counts
SELECT wallet_address, COUNT(*) as tx_count 
FROM transactions 
GROUP BY wallet_address;
```

## Example: Complete Workflow with Hardhat

```bash
# Terminal 1: Start Hardhat node
npx hardhat node

# Terminal 2: Run the TPS tool (first time to generate wallets)
cd go-tps
export RPC_URL="http://localhost:8545"
export WALLET_COUNT=5
export TX_PER_WALLET=5
./go-tps

# Note: The first run will generate a mnemonic and wallets
# Check mnemonic.txt for the generated phrase and wallet addresses

# Terminal 3: Fund the wallets using Hardhat's default funded account
# Example with cast (from foundry):
cast send <GENERATED_WALLET_ADDRESS> --value 1ether --private-key <HARDHAT_PRIVATE_KEY> --rpc-url http://localhost:8545

# Terminal 2: Run again with the same mnemonic after funding
MNEMONIC="$(cat mnemonic.txt | tail -n 1)" ./go-tps
```

## Common Configuration Examples

### High Volume Test
```bash
RPC_URL="http://localhost:8545" \
WALLET_COUNT=20 \
TX_PER_WALLET=20 \
./go-tps
```

### Small Test
```bash
RPC_URL="http://localhost:8545" \
WALLET_COUNT=3 \
TX_PER_WALLET=3 \
./go-tps
```

### Using Public Testnet (Sepolia)
```bash
RPC_URL="https://rpc.sepolia.org" \
WALLET_COUNT=5 \
TX_PER_WALLET=5 \
TO_ADDRESS="0xYourTestAddress" \
./go-tps
```

## Important Notes

1. **Security**: Never use real ETH or mainnet for testing!
2. **Gas Costs**: Each transaction requires gas. Budget accordingly.
3. **RPC Limits**: Public RPC endpoints may rate-limit you.
4. **Nonce Management**: The tool uses precalculated nonces, so run one instance at a time per wallet set.

## Troubleshooting

### "insufficient funds for gas * price + value"
Your wallets don't have enough ETH. Fund them first.

### "connection refused"
Your RPC endpoint isn't running or the URL is wrong.

### "nonce too low"
The wallet has already sent transactions. Database and blockchain state are out of sync. Use fresh wallets.

### "replacement transaction underpriced"
Multiple transactions with the same nonce. Ensure you're not running multiple instances simultaneously.

## Next Steps

After successful runs, analyze your transaction performance:
1. Check average execution times in the database
2. Compare TPS across different configurations
3. Test with different network conditions
4. Profile gas usage patterns

Happy testing! 🚀
