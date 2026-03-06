#!/usr/bin/env python3

import requests
import pandas as pd
from openpyxl import Workbook

RPC_URL = "https://eth-sepolia-internal-zeeve.blockinfra.zeeve.net/v1zhft4o8nxx8qvq6xj4/rpc"

tx_hashes = []

# read txn hashes from file
with open("tx_hashes.txt") as f:
    tx_hashes = [line.strip() for line in f.readlines()]

def rpc_call(method, params):
    payload = {
        "jsonrpc": "2.0",
        "method": method,
        "params": params,
        "id": 1
    }
    r = requests.post(RPC_URL, json=payload)
    return r.json()["result"]

rows = []

for tx in tx_hashes:
    try:
        receipt = rpc_call("eth_getTransactionReceipt", [tx])
        txn = rpc_call("eth_getTransactionByHash", [tx])

        gas_used = int(receipt["gasUsed"], 16)
        gas_price = int(txn["gasPrice"], 16)

        fee = gas_used * gas_price

        rows.append({
            "tx_hash": tx,
            "gas_used": gas_used,
            "gas_price_wei": gas_price,
            "tx_fee_eth": fee / 10**18
        })

    except Exception as e:
        print("error for tx:", tx, e)

df = pd.DataFrame(rows)
df.to_excel("txn_details.xlsx", index=False)

print("Excel file generated: txn_details.xlsx")