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

# Check if cast is available
if ! command -v cast &> /dev/null; then
    echo -e "${RED}ERROR: cast not found. Install Foundry first.${NC}"
    exit 1
fi

# Get starting block number
last_block=$(cast block-number --rpc-url "$RPC_URL")
echo "Starting from block #$last_block"
echo ""

while true; do
    current_block=$(cast block-number --rpc-url "$RPC_URL" 2>/dev/null || echo "$last_block")
    
    # Check if we have a new block
    if [ "$current_block" -gt "$last_block" ]; then
        # Process all new blocks
        for ((block=$((last_block+1)); block<=current_block; block++)); do
            timestamp=$(date '+%H:%M:%S')
            
            # Get transaction count for this block
            tx_count=$(cast rpc eth_getBlockTransactionCountByNumber "0x$(printf '%x' $block)" --rpc-url "$RPC_URL" 2>/dev/null | sed 's/^0x//' | xargs printf '%d\n' 2>/dev/null || echo "0")
            
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
            
            echo -e "[$timestamp] ${color}Block #$block: $tx_count transactions${NC}"
        done
        
        last_block=$current_block
    fi
    
    sleep 1
done