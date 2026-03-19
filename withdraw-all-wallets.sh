#!/bin/bash

# Withdraw all balances from multiple wallets back to target wallet
# Usage: ./withdraw-all-wallets.sh <JSON_FILE> <TARGET_WALLET_ADDRESS> <RPC_URL>

set -e

# Check arguments
if [ "$#" -ne 3 ]; then
    echo "Usage: $0 <JSON_FILE> <TARGET_WALLET_ADDRESS> <RPC_URL>"
    echo "Example: $0 wallets.json 0x1234567890123456789012345678901234567890 http://localhost:8545"
    exit 1
fi

JSON_FILE="$1"
TARGET_WALLET="$2"
RPC_URL="$3"

# Check if JSON file exists
if [ ! -f "$JSON_FILE" ]; then
    echo "Error: JSON file '$JSON_FILE' not found"
    exit 1
fi

# Check if jq is installed
if ! command -v jq &> /dev/null; then
    echo "Error: jq is required but not installed. Install with:"
    echo "  Ubuntu/Debian: sudo apt install jq"
    echo "  macOS: brew install jq"
    exit 1
fi

# Validate target wallet address format
if [[ ! "$TARGET_WALLET" =~ ^0x[a-fA-F0-9]{40}$ ]]; then
    echo "Error: Invalid target wallet address format"
    exit 1
fi

echo "=== WALLET BALANCE WITHDRAWAL ==="
echo "JSON file: $JSON_FILE"
echo "Target wallet: $TARGET_WALLET"
echo "RPC URL: $RPC_URL"
echo "======================================="

# Parse JSON file and extract wallet data using jq
echo "Parsing wallet data from JSON..."
WALLET_DATA=$(jq -r 'to_entries[] | "\(.key),\(.value.address),\(.value.privateKey)"' "$JSON_FILE")

if [ -z "$WALLET_DATA" ]; then
    echo "Error: No wallet data found in JSON file"
    exit 1
fi

# Count wallets
WALLET_COUNT=$(echo "$WALLET_DATA" | wc -l)
echo "Found $WALLET_COUNT wallets in JSON file"
echo ""

# Gas settings
GAS_LIMIT="30000"   # Standard ETH transfer
GAS_PRICE_GWEI="2"  # 2 gwei base price

# Get current gas price from network
echo "Fetching current gas price..."
NETWORK_GAS_PRICE=$(cast gas-price --rpc-url "$RPC_URL" 2>/dev/null || echo "2000000000")
NETWORK_GAS_PRICE_GWEI=$((NETWORK_GAS_PRICE / 1000000000))

# Use higher of configured or network gas price
if [ "$NETWORK_GAS_PRICE_GWEI" -gt "$GAS_PRICE_GWEI" ]; then
    GAS_PRICE_GWEI="$NETWORK_GAS_PRICE_GWEI"
fi

echo "Using gas price: ${GAS_PRICE_GWEI} gwei"

# Calculate gas cost in wei
GAS_COST_WEI=$((GAS_LIMIT * GAS_PRICE_GWEI * 1000000000))
echo "Gas cost per transaction: $GAS_COST_WEI wei ($(echo "scale=6; $GAS_COST_WEI / 1000000000000000000" | bc) ETH)"
echo ""

TOTAL_WITHDRAWN=0
SUCCESSFUL_WITHDRAWALS=0
FAILED_WITHDRAWALS=0

# Process each wallet
while IFS= read -r WALLET_LINE; do
    IFS=',' read -r WALLET_NAME ADDRESS PRIVATE_KEY <<< "$WALLET_LINE"
    
    echo "Processing $WALLET_NAME ($ADDRESS)..."
    
    # Get balance
    BALANCE_WEI=$(cast balance "$ADDRESS" --rpc-url "$RPC_URL" 2>/dev/null || echo "0")
    BALANCE_ETH=$(echo "scale=6; $BALANCE_WEI / 1000000000000000000" | bc)
    
    echo "  Balance: $BALANCE_WEI wei ($BALANCE_ETH ETH)"
    
    # Check if balance is sufficient for gas
    if [ "$BALANCE_WEI" -le "$GAS_COST_WEI" ]; then
        echo "  ⚠️  Skipping: Balance too low to cover gas costs"
        ((FAILED_WITHDRAWALS++))
        echo ""
        continue
    fi
    
    # Calculate amount to send (balance - gas cost)
    SEND_AMOUNT_WEI=$((BALANCE_WEI - GAS_COST_WEI))
    SEND_AMOUNT_ETH=$(echo "scale=6; $SEND_AMOUNT_WEI / 1000000000000000000" | bc)
    
    echo "  Sending: $SEND_AMOUNT_WEI wei ($SEND_AMOUNT_ETH ETH) to $TARGET_WALLET"
    
    # Send transaction
    TX_HASH=$(cast send \
        --private-key "$PRIVATE_KEY" \
        --rpc-url "$RPC_URL" \
        --gas-limit "$GAS_LIMIT" \
        --gas-price "${GAS_PRICE_GWEI}gwei" \
        --value "$SEND_AMOUNT_WEI" \
        "$TARGET_WALLET" \
        2>/dev/null)
    
    if [ $? -eq 0 ] && [ -n "$TX_HASH" ]; then
        echo "  ✅ Success! TX: $TX_HASH"
        TOTAL_WITHDRAWN=$((TOTAL_WITHDRAWN + SEND_AMOUNT_WEI))
        ((SUCCESSFUL_WITHDRAWALS++))
        
        # Wait a bit before next transaction
        sleep 1
    else
        echo "  ❌ Failed to send transaction"
        ((FAILED_WITHDRAWALS++))
    fi
    
    echo ""
done <<< "$WALLET_DATA"

# Summary
TOTAL_WITHDRAWN_ETH=$(echo "scale=6; $TOTAL_WITHDRAWN / 1000000000000000000" | bc)

echo "======================================="
echo "=== WITHDRAWAL SUMMARY ==="
echo "Successful withdrawals: $SUCCESSFUL_WITHDRAWALS"
echo "Failed withdrawals: $FAILED_WITHDRAWALS"
echo "Total withdrawn: $TOTAL_WITHDRAWN wei ($TOTAL_WITHDRAWN_ETH ETH)"
echo "Target wallet: $TARGET_WALLET"

# Final balance check of target wallet
echo ""
echo "Checking target wallet final balance..."
TARGET_BALANCE_WEI=$(cast balance "$TARGET_WALLET" --rpc-url "$RPC_URL" 2>/dev/null || echo "0")
TARGET_BALANCE_ETH=$(echo "scale=6; $TARGET_BALANCE_WEI / 1000000000000000000" | bc)
echo "Target wallet balance: $TARGET_BALANCE_WEI wei ($TARGET_BALANCE_ETH ETH)"

echo "======================================="
echo "✅ Withdrawal process completed!"