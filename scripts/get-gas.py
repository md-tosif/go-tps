#!/usr/bin/env python3
"""
Iterates through a specific sender's transactions by nonce range (derived from
START_TX_HASH and END_TX_HASH). Uses binary search on eth_getTransactionCount to
locate each transaction's block — no full block scanning of other users' txns.
"""

import requests
import pandas as pd

RPC_URL = "https://eth-sepolia-internal-zeeve.blockinfra.zeeve.net/v1zhft4o8nxx8qvq6xj4/rpc"

# --- Set the first and last transaction hashes (inclusive) ---
START_TX_HASH = "0x0af7376b579d49f886513e4eb2b1ddfae149f410ca6bd853b242556969bde49e"
END_TX_HASH   = "0xecc0f313a27c77bbd08035dce1f7d892a8cfb5f5f7bbb842666764e0b564c7c1"
# Sender is inferred automatically from START_TX_HASH
# -------------------------------------------------------------


def rpc_call(method, params):
    payload = {
        "jsonrpc": "2.0",
        "method": method,
        "params": params,
        "id": 1,
    }
    r = requests.post(RPC_URL, json=payload)
    return r.json()["result"]


def find_block_for_nonce(sender, nonce, block_min, block_max):
    """
    Binary search for the block where `sender`'s transaction with `nonce` was mined.
    eth_getTransactionCount(sender, block) returns how many txns the sender has
    confirmed up to that block (i.e. the next pending nonce).
    The tx with a given nonce is mined in the first block where count > nonce.
    """
    while block_min < block_max:
        mid = (block_min + block_max) // 2
        count = int(rpc_call("eth_getTransactionCount", [sender, hex(mid)]), 16)
        if count > nonce:
            block_max = mid
        else:
            block_min = mid + 1
    return block_min


def get_tx_from_block(sender, nonce, block_num):
    """Return the transaction object for sender+nonce from a specific block."""
    block = rpc_call("eth_getBlockByNumber", [hex(block_num), True])
    if block is None:
        return None
    for txn in block.get("transactions", []):
        if (txn["from"].lower() == sender.lower()
                and int(txn["nonce"], 16) == nonce):
            return txn
    return None


# --- Resolve sender and nonce range from the two anchor hashes ---
print("Resolving start and end transactions...")
start_tx = rpc_call("eth_getTransactionByHash", [START_TX_HASH])
end_tx   = rpc_call("eth_getTransactionByHash", [END_TX_HASH])

if start_tx is None or end_tx is None:
    raise ValueError("One or both transaction hashes not found on chain.")

if start_tx["from"].lower() != end_tx["from"].lower():
    raise ValueError("START_TX_HASH and END_TX_HASH must be sent by the same address.")

sender      = start_tx["from"]
nonce_start = int(start_tx["nonce"], 16)
nonce_end   = int(end_tx["nonce"], 16)
block_start = int(start_tx["blockNumber"], 16)
block_end   = int(end_tx["blockNumber"], 16)
total       = nonce_end - nonce_start + 1

print(f"Sender:      {sender}")
print(f"Nonce range: {nonce_start} → {nonce_end}  ({total} transactions)")
print(f"Block range: {block_start} → {block_end}")
print()

# --- Iterate only through this sender's transactions by nonce ---
rows = []
current_block_min = block_start  # progressive lower bound; nonces are ordered

for nonce in range(nonce_start, nonce_end + 1):
    print(f"  [{nonce - nonce_start + 1}/{total}] nonce {nonce} ...", end="\r")
    try:
        block_num = find_block_for_nonce(sender, nonce, current_block_min, block_end)
        current_block_min = block_num  # next nonce's block is always >= this one

        txn = get_tx_from_block(sender, nonce, block_num)
        if txn is None:
            print(f"\n  Warning: nonce {nonce} not found in block {block_num}, skipping.")
            continue

        receipt   = rpc_call("eth_getTransactionReceipt", [txn["hash"]])
        gas_used  = int(receipt["gasUsed"], 16)
        gas_price = int(txn["gasPrice"], 16)
        fee       = gas_used * gas_price

        rows.append({
            "tx_hash":       txn["hash"],
            "nonce":         nonce,
            "block_number":  block_num,
            "tx_index":      int(txn["transactionIndex"], 16),
            "gas_used":      gas_used,
            "gas_price_wei": gas_price,
            "tx_fee_eth":    fee / 10**18,
        })

    except Exception as e:
        print(f"\n  Error for nonce {nonce}: {e}")

print(f"\nProcessed {len(rows)} / {total} transactions.")

df = pd.DataFrame(rows)
df.to_excel("txn_details.xlsx", index=False)

print("Excel file generated: txn_details.xlsx")