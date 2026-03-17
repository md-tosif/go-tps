#!/bin/bash

# Simple Block Transaction Monitor
# Alternative simpler version using block number polling

set -euo pipefail

# Default RPC URL
RPC_URL="${RPC_URL:-http://localhost:8545}"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${BLUE}🔍 Simple Block Transaction Monitor${NC}"
echo "RPC URL: $RPC_URL"
echo "Press Ctrl+C to stop"
echo "--------------------------------"

# Check if required tools are available
if ! command -v curl &> /dev/null; then
    echo -e "${RED}ERROR: curl not found. Please install curl.${NC}"
    exit 1
fi

if ! command -v jq &> /dev/null; then
    echo -e "${RED}ERROR: jq not found. Please install jq for JSON parsing.${NC}"
    exit 1
fi

if ! command -v bc &> /dev/null; then
    echo -e "${RED}ERROR: bc not found. Please install bc for decimal calculations.${NC}"
    exit 1
fi

# Helper function to get latest block number
get_block_number() {
    curl -s -X POST -H "Content-Type: application/json" \
        --data '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' \
        "$RPC_URL" | jq -r '.result // "0x0"' 2>/dev/null | xargs -I {} printf "%d" {} 2>/dev/null || echo "0"
}

# Helper function to get transaction count for a block
get_tx_count() {
    local block_hex="$1"
    curl -s -X POST -H "Content-Type: application/json" \
        --data '{"jsonrpc":"2.0","method":"eth_getBlockTransactionCountByNumber","params":["'$block_hex'"],"id":1}' \
        "$RPC_URL" | jq -r '.result // "0x0"' 2>/dev/null | xargs -I {} printf "%d" {} 2>/dev/null || echo "0"
}

# Get starting block number
last_block=$(get_block_number)
echo "Starting from block #$last_block"
echo ""

while true; do
    current_block=$(get_block_number)
    
    # Check if we have a new block
    if [ "$current_block" -gt "$last_block" ]; then
        # Process all new blocks
        for ((block=$((last_block+1)); block<=current_block; block++)); do
            timestamp=$(date '+%H:%M:%S')
            block_hex="0x$(printf '%x' $block)"
            
            # Get transaction count for this block
            tx_count=$(get_tx_count "$block_hex")
            
            # Get block details with base fee
            block_data=$(curl -s -X POST -H "Content-Type: application/json" \
                --data '{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["'$block_hex'",false],"id":1}' \
                "$RPC_URL" 2>/dev/null || echo '{"result":{}}')
            
            # Extract base fee from JSON response
            base_fee_hex=$(echo "$block_data" | jq -r '.result.baseFeePerGas // "0x0"' 2>/dev/null || echo "0x0")
            
            # Convert base fee from hex to decimal (wei) and then to gwei
            if [ "$base_fee_hex" != "0x0" ] && [ "$base_fee_hex" != "null" ] && [ "$base_fee_hex" != "" ]; then
                base_fee_wei=$(printf "%d" "$base_fee_hex" 2>/dev/null || echo "0")
                if [ "$base_fee_wei" -gt 0 ]; then
                    base_fee_gwei=$(echo "scale=3; $base_fee_wei / 1000000000" | bc -l 2>/dev/null || echo "0.000")
                else
                    base_fee_gwei="0.000"
                fi
            else
                base_fee_gwei="0.000"
            fi
            
            # Color based on transaction count
            if [ "$tx_count" -eq 0 ]; then
                color="$YELLOW"
            elif [ "$tx_count" -lt 10 ]; then
                color="$GREEN"  
            elif [ "$tx_count" -lt 290 ]; then
                color="$BLUE"
            else
                color="$RED"
            fi
            
            echo -e "[$timestamp] ${color}Block #$block: $tx_count txs, Base Fee: ${base_fee_gwei} gwei${NC}"
        done
        
        last_block=$current_block
    fi
    
    sleep 1
done