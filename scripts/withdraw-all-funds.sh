#!/bin/bash

# Withdraw all funds from mnemonic-derived wallets
# Usage: ./withdraw-all-funds.sh <MNEMONIC> <DESTINATION_ADDRESS> [COUNT] [RPC_URL]

RPC_URL="${4:-http://localhost:8545}"
COUNT="${3:-100}"
MNEMONIC="$1"
DESTINATION="$2"

if [ -z "$MNEMONIC" ] || [ -z "$DESTINATION" ]; then
  echo "Usage: $0 <MNEMONIC> <DESTINATION_ADDRESS> [COUNT] [RPC_URL]"
  echo ""
  echo "Examples:"
  echo "  $0 \"abandon abandon abandon...\" 0x742d35Cc6639C0532fEb66FF7CB9132dcBacbD1A 20"
  echo "  $0 \"abandon abandon abandon...\" 0x742d35Cc6639C0532fEb66FF7CB9132dcBacbD1A 20 http://localhost:8545"
  echo ""
  echo "Parameters:"
  echo "  MNEMONIC          - BIP39 mnemonic phrase (12-24 words)"
  echo "  DESTINATION       - Address to send all funds to"
  echo "  COUNT             - Number of wallets to check (default: 20)"
  echo "  RPC_URL           - Ethereum RPC endpoint (default: http://localhost:8545)"
  exit 1
fi

echo "=== WITHDRAWING FUNDS FROM MNEMONIC WALLETS ==="
echo "Destination: $DESTINATION"
echo "Wallet Count: $COUNT"
echo "RPC URL: $RPC_URL"
echo ""

# Get current gas price
echo "Getting current gas price..."
GAS_PRICE=$(cast gas-price --rpc-url "$RPC_URL" 2>/dev/null)
if [ -z "$GAS_PRICE" ]; then
  echo "⚠️  Could not get gas price from RPC, using 20 gwei default"
  GAS_PRICE="20000000000"  # 20 gwei in wei
else
  echo "Current gas price: $GAS_PRICE wei"
fi

# Gas limit for ETH transfer
GAS_LIMIT=21000

# Calculate gas cost in wei
GAS_COST=$(echo "$GAS_PRICE * $GAS_LIMIT" | bc)
echo "Gas cost per transaction: $GAS_COST wei"
echo ""

TOTAL_WITHDRAWN=0
SUCCESSFUL_WITHDRAWALS=0
FAILED_WITHDRAWALS=0
EMPTY_WALLETS=0

for i in $(seq 0 $((COUNT-1)))
do
  echo "Processing wallet[$i]..."
  
  # Get wallet address and private key
  WALLET_ADDR=$(cast wallet address --mnemonic "$MNEMONIC" --mnemonic-index $i 2>/dev/null)
  if [ -z "$WALLET_ADDR" ]; then
    echo "  ❌ Failed to derive wallet address"
    ((FAILED_WITHDRAWALS++))
    continue
  fi
  
  # Get wallet balance
  BALANCE=$(cast balance "$WALLET_ADDR" --rpc-url "$RPC_URL" 2>/dev/null)
  if [ -z "$BALANCE" ]; then
    echo "  ❌ Failed to get balance for $WALLET_ADDR"
    ((FAILED_WITHDRAWALS++))
    continue
  fi
  
  echo "  Address: $WALLET_ADDR"
  echo "  Balance: $BALANCE wei ($(cast to-unit $BALANCE ether) ETH)"
  
  # Use cast to estimate actual gas cost (more accurate than manual calculation)
  echo "  Estimating gas cost..."
  ESTIMATED_GAS_COST=$(cast estimate --from "$WALLET_ADDR" --to "$DESTINATION" --value "1" --rpc-url "$RPC_URL" 2>/dev/null)
  
  if [ -n "$ESTIMATED_GAS_COST" ]; then
    # Get current gas price and calculate total cost
    CURRENT_GAS_PRICE=$(cast gas-price --rpc-url "$RPC_URL" 2>/dev/null || echo "$GAS_PRICE")
    TOTAL_GAS_COST=$(echo "$CURRENT_GAS_PRICE * $ESTIMATED_GAS_COST" | bc | cut -d. -f1)
    # Add 50% safety buffer for EIP-1559 and gas fluctuations  
    SAFE_GAS_COST=$(echo "$TOTAL_GAS_COST * 1.5" | bc | cut -d. -f1)
  else
    echo "  ⚠️  Could not estimate gas, using conservative fixed amount"
    # Use fixed 0.01 ETH (10^16 wei) for gas - very conservative
    SAFE_GAS_COST="10000000000000000"
  fi
  
  # Check if wallet has enough funds to cover gas
  if [ "$(echo "$BALANCE <= $SAFE_GAS_COST" | bc)" -eq "1" ]; then
    if [ "$(echo "$BALANCE == 0" | bc)" -eq "1" ]; then
      echo "  ⭕ Empty wallet, skipping"
      ((EMPTY_WALLETS++))
    else
      echo "  ⚠️  Balance too low to cover gas costs, skipping (needs $(cast to-unit $SAFE_GAS_COST ether) ETH for gas)"
      ((FAILED_WITHDRAWALS++))
    fi
    continue
  fi
  
  # Calculate amount to send (balance minus safe gas cost)
  SEND_AMOUNT=$(echo "$BALANCE - $SAFE_GAS_COST" | bc | cut -d. -f1)
  echo "  Withdrawing: $SEND_AMOUNT wei ($(cast to-unit $SEND_AMOUNT ether) ETH)"
  
  # Get private key for this wallet
  WALLET_PK=$(cast wallet private-key --mnemonic "$MNEMONIC" --mnemonic-index $i 2>/dev/null)
  if [ -z "$WALLET_PK" ]; then
    echo "  ❌ Failed to derive private key"
    ((FAILED_WITHDRAWALS++))
    continue
  fi
  
  # Send transaction (let cast handle gas price automatically for better reliability)
  if TX_HASH=$(cast send --private-key "$WALLET_PK" \
                        --rpc-url "$RPC_URL" \
                        --gas-limit "$GAS_LIMIT" \
                        --value "$SEND_AMOUNT" \
                        "$DESTINATION" \
                        2>&1); then
    TX_HASH_CLEAN=$(echo "$TX_HASH" | grep -oP '0x[a-fA-F0-9]{64}' | head -1)
    echo "  ✅ Success! TX: $TX_HASH_CLEAN"
    TOTAL_WITHDRAWN=$(echo "$TOTAL_WITHDRAWN + $SEND_AMOUNT" | bc)
    ((SUCCESSFUL_WITHDRAWALS++))
  else
    echo "  ❌ Failed: $TX_HASH"
    ((FAILED_WITHDRAWALS++))
  fi
  
  echo ""
  sleep 1
done

echo "=== WITHDRAWAL SUMMARY ==="
echo "Total wallets processed: $COUNT"
echo "Successful withdrawals: $SUCCESSFUL_WITHDRAWALS"
echo "Failed withdrawals: $FAILED_WITHDRAWALS"
echo "Empty wallets: $EMPTY_WALLETS"
echo "Total amount withdrawn: $TOTAL_WITHDRAWN wei ($(cast to-unit $TOTAL_WITHDRAWN ether) ETH)"
echo "Destination address: $DESTINATION"
echo ""
echo "✅ Withdrawal process completed!"