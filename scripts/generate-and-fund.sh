#!/bin/bash

RPC_URL="http://localhost:8545"
COUNT=20
AMOUNT="10ether"

FUNDER_PK=$1

if [ -z "$FUNDER_PK" ]; then
  echo "Usage: $0 <FUNDER_PRIVATE_KEY>"
  exit 1
fi

# generate mnemonic
MNEMONIC=$(cast wallet new-mnemonic)

echo "Generated Mnemonic:"
echo "$MNEMONIC"
echo ""

for i in $(seq 0 $((COUNT-1)))
do
  ADDR=$(cast wallet address --mnemonic "$MNEMONIC" --mnemonic-index $i)

  echo "Funding wallet[$i] -> $ADDR"

  if TX_HASH=$(cast send --private-key "$FUNDER_PK" \
                        --rpc-url "$RPC_URL" \
                        --value "$AMOUNT" \
                        "$ADDR" \
                        2>&1); then
    echo "    ✅ Success! TX: $(echo "$TX_HASH" | grep -oP '0x[a-fA-F0-9]{64}' | head -1)"
  else
    echo "    ❌ Failed: $TX_HASH"
  fi

  sleep 1
done

echo ""
echo "Finished funding wallets."