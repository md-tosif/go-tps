#!/bin/bash

# Fund wallets script - Sends 10 ETH to each wallet
# Usage: ./fund_wallets.sh <PRIVATE_KEY_OF_SENDER>

set -e

# Configuration
RPC_URL="http://46.165.235.105:8545"
AMOUNT="10ether"

# Wallet addresses (from go-tps output)
WALLETS=(
    "0xC354e95293751d4d65A0c4ed6F9ddeb90f13B05B"
    "0xbd4979DF3A206e81F6cF0614465E61fB025669AB"
    "0x25eB87E85a300C4fAE7dA18737349Aa176C3ba31"
    "0x5DB5fe01866f045BDbB00EEE3FF0647c04c322A5"
    "0x9384F6a3554d31E54C2E3EFFd81B11D313e9A5cd"
    "0x8480a854fde1FB802D490cC8a0b7Ee52c793B482"
    "0xeF8543a23119Bbf284A57835b30F351476279E2b"
    "0x109e70A15A8682Ab4A3eDb20EeDCcb61F7078Cd3"
    "0x5d66C7854F374BAB2D82F6cC0ac90F00798eAFd3"
    "0x6e36d999337F175094B496e0A2A19bf925a85987"
    "0x06d9a9CE01275AF62b6f60B16e371671D83f71A1"
    "0x86afD7D776FA2476A1A79e67F73D60D3193e8779"
    "0x1104c41d7127c8175662070bb2b345261809D6DF"
    "0xC6F904d59D0f48e900461550d20Eb406A30277ff"
    "0x29A6794FB88846C27a7bceFE47C8B07dDb00f695"
    "0x9413D56E7d2d30371F42D7F7A92cfD79Ea229c63"
    "0x7f3352EDD23098f3c8a217627292000785E42e21"
    "0xc3Af823b46757678Df0DF516a485291958Bc3468"
    "0xA5A45d39C54b43587bfbb133811eC80E3f7bd1DB"
    "0x3d0B2E6101E615f1d5024a94aC12026599df5a1D"
)

# Check for private key argument
if [ -z "$1" ]; then
    echo "❌ Error: Private key required"
    echo ""
    echo "Usage: $0 <PRIVATE_KEY_OF_SENDER>"
    echo ""
    echo "Example:"
    echo "  $0 0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
    echo ""
    echo "Or use environment variable:"
    echo "  PRIVATE_KEY=0x... $0"
    exit 1
fi

PRIVATE_KEY="$1"

# Verify cast is installed
if ! command -v cast &> /dev/null; then
    echo "❌ Error: 'cast' command not found"
    echo "Please install Foundry: https://book.getfoundry.sh/getting-started/installation"
    exit 1
fi

echo "============================================================"
echo "                 FUNDING WALLETS WITH 10 ETH"
echo "============================================================"
echo "RPC URL: $RPC_URL"
echo "Amount per wallet: $AMOUNT"
echo "Total wallets: ${#WALLETS[@]}"
echo "Total ETH needed: $((${#WALLETS[@]} * 10)) ETH"
echo "============================================================"
echo ""

# Check sender balance
SENDER_ADDRESS=$(cast wallet address "$PRIVATE_KEY")
SENDER_BALANCE=$(cast balance "$SENDER_ADDRESS" --rpc-url "$RPC_URL")
SENDER_BALANCE_ETH=$(cast to-unit "$SENDER_BALANCE" ether)

echo "Sender Address: $SENDER_ADDRESS"
echo "Sender Balance: $SENDER_BALANCE_ETH ETH"
echo ""

# Confirm before proceeding
read -p "Proceed with funding? (y/n): " -n 1 -r
echo ""
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "❌ Aborted by user"
    exit 1
fi

echo ""
echo "🚀 Starting funding process..."
echo ""

# Fund each wallet
SUCCESS_COUNT=0
FAILED_COUNT=0

for i in "${!WALLETS[@]}"; do
    WALLET="${WALLETS[$i]}"
    WALLET_NUM=$((i + 1))
    
    echo "[$WALLET_NUM/${#WALLETS[@]}] Funding $WALLET..."
    
    # Send transaction
    if TX_HASH=$(cast send --private-key "$PRIVATE_KEY" \
                          --rpc-url "$RPC_URL" \
                          --value "$AMOUNT" \
                          "$WALLET" \
                          2>&1); then
        echo "    ✅ Success! TX: $(echo "$TX_HASH" | grep -oP '0x[a-fA-F0-9]{64}' | head -1)"
        SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
    else
        echo "    ❌ Failed: $TX_HASH"
        FAILED_COUNT=$((FAILED_COUNT + 1))
    fi
    
    # Small delay to avoid overwhelming RPC
    sleep 0.5
done

echo ""
echo "============================================================"
echo "                    FUNDING COMPLETE"
echo "============================================================"
echo "✅ Successful: $SUCCESS_COUNT"
echo "❌ Failed: $FAILED_COUNT"
echo "============================================================"

# Show updated balances
echo ""
echo "Checking updated balances..."
echo ""

for i in "${!WALLETS[@]}"; do
    WALLET="${WALLETS[$i]}"
    WALLET_NUM=$((i + 1))
    BALANCE=$(cast balance "$WALLET" --rpc-url "$RPC_URL")
    BALANCE_ETH=$(cast to-unit "$BALANCE" ether)
    printf "[$WALLET_NUM] %s: %s ETH\n" "$WALLET" "$BALANCE_ETH"
done

echo ""
echo "✨ All done!"
