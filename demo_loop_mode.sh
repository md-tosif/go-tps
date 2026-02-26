#!/bin/bash

# Demo script for Loop Mode feature
# This demonstrates how to run the TPS tester in loop mode

echo "=== Go-TPS Loop Mode Demo ==="
echo ""
echo "This script demonstrates the loop mode feature."
echo "Loop mode allows continuous testing for a specified duration."
echo ""

# Example 1: Single run (default)
echo "Example 1: Single Run (default behavior)"
echo "Command: RUN_DURATION_MINUTES=0 ./go-tps"
echo ""

# Example 2: Loop for 2 minutes
echo "Example 2: Loop Mode - Run for 2 minutes"
echo "Command: RUN_DURATION_MINUTES=2 WALLET_COUNT=2 TX_PER_WALLET=5 ./go-tps"
echo ""
echo "In loop mode, you'll see:"
echo "  - Loop started at: HH:MM:SS"
echo "  - Will run until: HH:MM:SS"
echo "  - [ITERATION #1] Time remaining: X.X minutes"
echo "  - ... transaction processing ..."
echo "  - ✓ Iteration complete. Starting next iteration..."
echo "  - [ITERATION #2] Time remaining: X.X minutes"
echo "  - ... and so on until time expires"
echo ""

# Example 3: Long duration test
echo "Example 3: Extended Load Test - Run for 30 minutes"
echo "Command: RUN_DURATION_MINUTES=30 WALLET_COUNT=10 TX_PER_WALLET=20 ./go-tps"
echo ""

# Example 4: Using .env file
echo "Example 4: Using .env file"
echo "Create a .env file with:"
echo "  RUN_DURATION_MINUTES=5"
echo "  WALLET_COUNT=5"
echo "  TX_PER_WALLET=10"
echo ""
echo "Then run: ./go-tps"
echo ""

echo "=== Key Features of Loop Mode ==="
echo ""
echo "✓ Continuous testing for specified duration (in minutes)"
echo "✓ Multiple iterations within the time window"
echo "✓ All data stored in the same database (cumulative)"
echo "✓ Shows iteration count and remaining time"
echo "✓ 2-second delay between iterations"
echo "✓ Graceful completion when time expires"
echo ""

echo "=== Analyzing Results After Loop Mode ==="
echo ""
echo "After running in loop mode, use the analysis tools:"
echo ""
echo "  ./analyze.sh summary    # Overall statistics"
echo "  ./analyze.sh tps        # TPS metrics"
echo "  ./analyze.sh timeline   # Performance over time"
echo ""

echo "=== Run a Quick Test? ==="
echo ""
read -p "Run a 1-minute loop mode test? (y/n): " -n 1 -r
echo ""

if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo ""
    echo "Starting 1-minute loop test with minimal load..."
    echo "Press Ctrl+C to cancel"
    echo ""
    sleep 2
    
    RUN_DURATION_MINUTES=1 \
    WALLET_COUNT=1 \
    TX_PER_WALLET=3 \
    ./go-tps
    
    echo ""
    echo "Test complete! Check the database:"
    ./analyze.sh summary
else
    echo "Demo complete. Try the commands above to test loop mode yourself!"
fi
