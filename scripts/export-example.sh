#!/bin/bash

# Example script showing how to use the address transaction exporter
# This demonstrates exporting transactions for some well-known Ethereum addresses

set -e

echo "=== Ethereum Address Transaction Export Examples ==="
echo

# Check if API key is set
if [[ -z "$ETHERSCAN_API_KEY" ]]; then
    echo "❌ Error: Please set your Etherscan API key"
    echo "Get a free API key from: https://etherscan.io/apis"
    echo "Then run: export ETHERSCAN_API_KEY=your_key_here"
    echo
    exit 1
fi

# Create exports directory
mkdir -p exports
export OUTPUT_DIR=./exports

echo "📂 Exports will be saved to: ./exports/"
echo

# Example addresses (well-known contracts/addresses)
declare -a addresses=(
    "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045"  # vitalik.eth
    "0xfB6916095ca1df60bB79Ce92cE3Ea74c37c5d359"  # Some active address
)

declare -a names=(
    "vitalik.eth"
    "active-address"
)

echo "Select an example address to export:"
for i in "${!addresses[@]}"; do
    echo "  $((i+1)). ${names[i]} (${addresses[i]})"
done
echo "  0. Use custom address"
echo

read -p "Enter your choice (0-${#addresses[@]}): " choice

case $choice in
    0)
        read -p "Enter Ethereum address (0x...): " custom_address
        if [[ ! "$custom_address" =~ ^0x[a-fA-F0-9]{40}$ ]]; then
            echo "❌ Invalid address format"
            exit 1
        fi
        target_address="$custom_address"
        target_name="custom"
        ;;
    [1-2])
        index=$((choice-1))
        target_address="${addresses[index]}"
        target_name="${names[index]}"
        ;;
    *)
        echo "❌ Invalid choice"
        exit 1
        ;;
esac

echo
echo "🎯 Selected address: $target_address ($target_name)"
echo

# Ask about internal transactions
read -p "Include internal transactions? (y/N): " include_internal
if [[ "$include_internal" =~ ^[Yy]$ ]]; then
    internal_flag="--include-internal"
    echo "✅ Will include internal transactions"
else
    internal_flag=""
    echo "ℹ️  Will export normal transactions only"
fi

echo
echo "🚀 Starting export..."
echo "⏱️  This may take a few minutes for addresses with many transactions..."
echo

# Execute the export
node scripts/export-address-transactions.js $internal_flag "$target_address"

echo
echo "🎉 Export completed!"
echo "📁 Check the exports/ directory for your CSV file"

# List created files
echo
echo "📋 Files created:"
ls -la exports/*.csv 2>/dev/null | tail -5 || echo "  No CSV files found"