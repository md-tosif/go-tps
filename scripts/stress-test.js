#!/usr/bin/env node
import { ethers } from "ethers";
import fs from "fs";
import path from "path";
 
const RPC_URL = process.env.RPC_URL || "http://localhost:8545";
const CHAIN_ID = Number(process.env.CHAIN_ID || "2027");
const FUND_PK = process.env.FUND_PK || "0xca58c33a697b16e3aab83ecdf74c794df0db8fa590338a51b94cfe2d5d4b4c66";
const TXNS_PER_WALLET = Number(process.env.TXNS_PER_WALLET || "1200");
const WALLET_COUNT = Number(process.env.WALLET_COUNT || "20");
const WALLET_OFFSET = Number(process.env.WALLET_OFFSET || "0");
const MNEMONIC = process.env.MNEMONIC || "soup always observe bitter squirrel grape drip remain when expand kite either";
const VALUE = ethers.parseEther(process.env.VALUE_ETH || "0");
const FUND_THRESHOLD = ethers.parseEther(process.env.FUND_THRESHOLD_ETH || "0.08");
const FUND_AMOUNT = ethers.parseEther(process.env.FUND_AMOUNT_ETH || "10");
const SIGNED_OUTPUT = process.argv[2] || "signed-txns.json";
const USE_RPC_BATCH = process.env.RPC_BATCH !== "1"; // set RPC_BATCH=0 to send requests individually
const MAX_FEE_PER_GAS = process.env.MAX_FEE_PER_GAS_GWEI ? ethers.parseUnits(process.env.MAX_FEE_PER_GAS_GWEI, "gwei") : null;
const MAX_PRIORITY_FEE_PER_GAS = process.env.MAX_PRIORITY_FEE_PER_GAS_GWEI ? ethers.parseUnits(process.env.MAX_PRIORITY_FEE_PER_GAS_GWEI, "gwei") : null;
 
const provider = new ethers.JsonRpcProvider(RPC_URL);
const funder = new ethers.Wallet(FUND_PK, provider);
 
const deriveWalletKeys = () => {
  if (!MNEMONIC) {
    console.error("MNEMONIC env is required for deterministic wallet derivation.");
    process.exit(1);
  }
  const keys = [];
  for (let i = 0; i < WALLET_COUNT; i += 1) {
    const idx = WALLET_OFFSET + i;
    const wallet = ethers.HDNodeWallet.fromPhrase(MNEMONIC, undefined, `m/44'/60'/0'/0/${idx}`);
    keys.push(wallet.privateKey);
  }
  return keys;
};
 
const walletKeys = deriveWalletKeys();
 
const fundIfNeeded = async (wallet) => {
  const balance = await provider.getBalance(wallet.address);
  console.log(`Wallet ${wallet.address} balance: ${ethers.formatEther(balance)} ETH (threshold: ${ethers.formatEther(FUND_THRESHOLD)} ETH)`);
  if (balance >= FUND_THRESHOLD) {
    return;
  }
  console.log(`Funding ${wallet.address} with ${ethers.formatEther(FUND_AMOUNT)} ETH...`);
  const tx = await funder.sendTransaction({
    to: wallet.address,
    value: FUND_AMOUNT,
    gasLimit: 21000,
  });
  await tx.wait();
  const newBalance = await provider.getBalance(wallet.address);
  console.log(`New balance for ${wallet.address}: ${ethers.formatEther(newBalance)} ETH`);
};
 
const prepareSignedTransactions = async () => {
  const signed = [];
 
  for (const pk of walletKeys) {
    const wallet = new ethers.Wallet(pk, provider);
    await fundIfNeeded(wallet);
    const startNonce = await provider.getTransactionCount(wallet.address);
 
    const feeData = await provider.getFeeData();
    const maxFee = MAX_FEE_PER_GAS ?? feeData.maxFeePerGas ?? ethers.parseUnits("2", "gwei");
    const maxPriority = MAX_PRIORITY_FEE_PER_GAS ?? feeData.maxPriorityFeePerGas ?? ethers.parseUnits("1", "gwei");
 
    for (let i = 0; i < TXNS_PER_WALLET; i += 1) {
      const tx = {
        to: funder.address,
        value: VALUE,
        nonce: startNonce + i,
        gasLimit: 21000,
        maxFeePerGas: maxFee,
        maxPriorityFeePerGas: maxPriority,
        chainId: CHAIN_ID,
      };
      signed.push(await wallet.signTransaction(tx));
    }
  }
 
  await fs.promises.writeFile(SIGNED_OUTPUT, JSON.stringify(signed, null, 2));
  return signed;
};
 
const submitAll = async (signedTxns) => {
  console.log(`Dispatching ${signedTxns.length} signed transactions${USE_RPC_BATCH ? " via JSON-RPC batch" : ""}...`);
  // print start time in ms
  console.log(`Start time: ${Date.now()} ms`);
  // print start block number
  const startBlock = await provider.getBlockNumber();
  console.log(`Start block: ${startBlock}`);
  if (USE_RPC_BATCH) {
    const payload = signedTxns.map((raw, idx) => ({
      jsonrpc: "2.0",
      id: idx + 1,
      method: "eth_sendRawTransaction",
      params: [raw],
    }));
 
    const res = await fetch(RPC_URL, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(payload),
    });
 
    const body = await res.json();
    const failures = body.filter((r) => r.error);
    if (failures.length) {
      console.error(`Batch submission had ${failures.length} failures. First error:`, failures[0].error);
      throw new Error("Some transactions failed in batch submission");
    }
  } else {
    const txns = signedTxns.map((raw) => provider.send("eth_sendRawTransaction", [raw]));
    await Promise.all(txns);
  }
  console.log(`All transactions dispatched at: ${Date.now()} ms`);
  // print end block number
  const endBlock = await provider.getBlockNumber();
  console.log(`End block: ${endBlock}`);

  return {endBlock, startBlock}
};

const printBlocksAndTransactions = async (startBlock, endBlock) => {
  console.log(`\n=== Block Analysis (${startBlock} to ${endBlock}) ===`);
  
  for (let blockNum = startBlock; blockNum <= endBlock; blockNum++) {
    try {
      const block = await provider.getBlock(blockNum, true); // true to include full transaction details
      if (!block) {
        console.log(`Block ${blockNum}: Not found`);
        continue;
      }
      
      console.log(`\nBlock ${blockNum}:`);
      console.log(`  Hash: ${block.hash}`);
      console.log(`  Timestamp: ${new Date(block.timestamp * 1000).toISOString()}`);
      console.log(`  Transaction Count: ${block.transactions.length}`);
      console.log(`  Gas Used: ${block.gasUsed.toString()} / ${block.gasLimit.toString()} (${((Number(block.gasUsed) / Number(block.gasLimit)) * 100).toFixed(2)}%)`);
      
    } catch (error) {
      console.error(`Error fetching block ${blockNum}:`, error.message);
    }
  }
  console.log(`\n=== End Block Analysis ===\n`);
};
 
const main = async () => {
  console.log(`RPC  : ${RPC_URL}`);
  console.log(`Chain: ${CHAIN_ID}`);
  console.log(`Signer: ${funder.address}`);
 
  const signed = await prepareSignedTransactions();
  console.log(`Prepared ${signed.length} signed transactions. Saving to ${SIGNED_OUTPUT}`);
 
  console.log("Sending all signed transactions in parallel...");
  const {startBlock, endBlock} = await submitAll(signed);
  
  console.log("All transactions submitted.");
  
  // Print block analysis
  await printBlocksAndTransactions(startBlock, endBlock);
};
 
main().catch((err) => {
  console.error(err);
  process.exit(1);
});
 
 