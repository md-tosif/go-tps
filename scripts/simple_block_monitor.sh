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

# Helper function to extract JSON result field using sed
extract_json_result() {
    sed -n 's/.*"result":"\([^"]*\)".*/\1/p' | head -1
}

# Helper function to convert hex to decimal
hex_to_dec() {
    local hex_val="$1"
    if [[ "$hex_val" =~ ^0x[0-9a-fA-F]+$ ]]; then
        printf "%d" "$hex_val" 2>/dev/null || echo "0"
    else
        echo "0"
    fi
}

# Helper function to get latest block number
get_block_number() {
    local result=$(curl -s -X POST -H "Content-Type: application/json" \
        --data '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' \
        "$RPC_URL" | extract_json_result)
    hex_to_dec "$result"
}

# Helper function to get transaction count for a block
get_tx_count() {
    local block_hex="$1"
    local result=$(curl -s -X POST -H "Content-Type: application/json" \
        --data '{"jsonrpc":"2.0","method":"eth_getBlockTransactionCountByNumber","params":["'$block_hex'"],"id":1}' \
        "$RPC_URL" | extract_json_result)
    hex_to_dec "$result"
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
            
            # Parallel HTTP requests for better performance
            (
                # Get transaction count in background
                tx_count=$(get_tx_count "$block_hex")
                echo "TX_COUNT:$tx_count" > /tmp/block_${block}_tx.tmp
            ) &
            
            (
                # Get block details in background  
                block_data=$(curl -s -X POST -H "Content-Type: application/json" \
                    --data '{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["'$block_hex'",false],"id":1}' \
                    "$RPC_URL" 2>/dev/null || echo '{"result":{}}')
                echo "$block_data" > /tmp/block_${block}_data.tmp
            ) &
            
            # Wait for both background jobs to complete
            wait
            
            # Read results from temp files
            tx_count=$(cat /tmp/block_${block}_tx.tmp | cut -d: -f2)
            block_data=$(cat /tmp/block_${block}_data.tmp)
            
            # Clean up temp files
            rm -f /tmp/block_${block}_tx.tmp /tmp/block_${block}_data.tmp
            
            # Extract base fee from JSON response using sed
            base_fee_hex=$(echo "$block_data" | sed -n 's/.*"baseFeePerGas":"\([^"]*\)".*/\1/p' | head -1)
            
            # Convert base fee from hex to decimal (wei) and then to gwei using bash arithmetic
            if [ -n "$base_fee_hex" ] && [ "$base_fee_hex" != "null" ] && [ "$base_fee_hex" != "" ]; then
                base_fee_wei=$(hex_to_dec "$base_fee_hex")
                if [ "$base_fee_wei" -gt 0 ]; then
                    # Convert to gwei using bash arithmetic (limited precision)
                    base_fee_gwei=$((base_fee_wei / 1000000000))
                    base_fee_remainder=$((base_fee_wei % 1000000000))
                    # Format with basic precision (3 decimal places)
                    base_fee_gwei_decimal=$((base_fee_remainder / 1000000))
                    base_fee_display="${base_fee_wei} wei (${base_fee_gwei}.$(printf "%03d" $base_fee_gwei_decimal) gwei)"
                else
                    base_fee_display="0 wei (0.000 gwei)"
                fi
            else
                base_fee_display="0 wei (0.000 gwei)"
            fi
            
            # Color based on transaction count
            if [ "$tx_count" -eq 100 ]; then
                color="$RED"
                (printf "\a\a\a") &  # Triple beep in background
            elif [ "$tx_count" -lt 150 ]; then
                color="$YELLOW"
                (printf "\a") &  # Single beep in background
            elif [ "$tx_count" -lt 252 ]; then
                color="$GREEN"  
            else
                color="$BLUE"
            fi
            
            echo -e "[$timestamp] ${color}Block #$block: $tx_count txs, Base Fee: ${base_fee_display}${NC}"
        done
        
        last_block=$current_block
    fi
    
    sleep 0.5
done