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

# First pass: Check which wallets have sufficient balance
echo ""
echo "Checking wallet balances..."
WALLETS_WITH_BALANCE=""
WALLETS_WITHOUT_BALANCE=""

while IFS= read -r WALLET_LINE; do
    IFS=',' read -r WALLET_NAME ADDRESS PRIVATE_KEY <<< "$WALLET_LINE"
    
    # Get balance
    BALANCE_WEI=$(cast balance "$ADDRESS" --rpc-url "$RPC_URL" 2>/dev/null || echo "0")
    BALANCE_ETH=$(echo "scale=6; $BALANCE_WEI / 1000000000000000000" | bc)
    
    echo "  [$WALLET_NAME] $BALANCE_WEI wei ($BALANCE_ETH ETH)"
    
    # Check if balance is sufficient for gas
    if [ "$BALANCE_WEI" -le "$GAS_COST_WEI" ]; then
        WALLETS_WITHOUT_BALANCE="$WALLETS_WITHOUT_BALANCE$WALLET_LINE\n"
    else
        WALLETS_WITH_BALANCE="$WALLETS_WITH_BALANCE$WALLET_LINE\n"
    fi
done <<< "$WALLET_DATA"

# Remove trailing newlines and count
WALLETS_WITH_BALANCE=$(echo -e "$WALLETS_WITH_BALANCE" | sed '/^$/d')
WALLETS_WITHOUT_BALANCE=$(echo -e "$WALLETS_WITHOUT_BALANCE" | sed '/^$/d')

WALLETS_WITH_BALANCE_COUNT=0
if [ -n "$WALLETS_WITH_BALANCE" ]; then
    WALLETS_WITH_BALANCE_COUNT=$(echo "$WALLETS_WITH_BALANCE" | wc -l)
fi

WALLETS_WITHOUT_BALANCE_COUNT=0
if [ -n "$WALLETS_WITHOUT_BALANCE" ]; then
    WALLETS_WITHOUT_BALANCE_COUNT=$(echo "$WALLETS_WITHOUT_BALANCE" | wc -l)
fi

echo ""
echo "Summary:"
echo "  Wallets with sufficient balance: $WALLETS_WITH_BALANCE_COUNT"
echo "  Wallets with insufficient balance: $WALLETS_WITHOUT_BALANCE_COUNT"

if [ "$WALLETS_WITH_BALANCE_COUNT" -eq 0 ]; then
    echo ""
    echo "❌ No wallets have sufficient balance to transfer. Exiting."
    exit 0
fi

echo ""
echo "Processing only wallets with sufficient balance..."
echo "======================================="

TOTAL_WITHDRAWN=0
SUCCESSFUL_WITHDRAWALS=0
FAILED_WITHDRAWALS=0

# Process only wallets with sufficient balance
while IFS= read -r WALLET_LINE; do
    IFS=',' read -r WALLET_NAME ADDRESS PRIVATE_KEY <<< "$WALLET_LINE"
    
    echo "Processing $WALLET_NAME ($ADDRESS)..."
    
    # Get current balance (may have changed since first check)
    BALANCE_WEI=$(cast balance "$ADDRESS" --rpc-url "$RPC_URL" 2>/dev/null || echo "0")
    BALANCE_ETH=$(echo "scale=6; $BALANCE_WEI / 1000000000000000000" | bc)
    
    echo "  Balance: $BALANCE_WEI wei ($BALANCE_ETH ETH)"
    
    # Double-check balance is still sufficient (in case it changed)
    if [ "$BALANCE_WEI" -le "$GAS_COST_WEI" ]; then
        echo "  ⚠️  Balance changed since check - now insufficient for gas"
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
done <<< "$WALLETS_WITH_BALANCE"

# Summary
TOTAL_WITHDRAWN_ETH=$(echo "scale=6; $TOTAL_WITHDRAWN / 1000000000000000000" | bc)

echo "======================================="
echo "=== WITHDRAWAL SUMMARY ==="
echo "Total wallets found: $WALLET_COUNT"
echo "Wallets with sufficient balance: $WALLETS_WITH_BALANCE_COUNT"
echo "Wallets with insufficient balance: $WALLETS_WITHOUT_BALANCE_COUNT"
echo ""
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

# Show skipped wallets if any
if [ "$WALLETS_WITHOUT_BALANCE_COUNT" -gt 0 ]; then
    echo ""
    echo "Wallets skipped (insufficient balance for gas):"
    while IFS= read -r WALLET_LINE; do
        IFS=',' read -r WALLET_NAME ADDRESS PRIVATE_KEY <<< "$WALLET_LINE"
        echo "  - $WALLET_NAME ($ADDRESS)"
    done <<< "$WALLETS_WITHOUT_BALANCE"
fi

echo ""
echo "======================================="
echo "✅ Withdrawal process completed!"