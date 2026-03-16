#!/usr/bin/env node

const https = require('https');
const fs = require('fs');
const path = require('path');

// Configuration
const ETHERSCAN_API_KEY = process.env.ETHERSCAN_API_KEY || 'UFW1JQ9NGAEWHSN7DASEKSS1N6WTTHHFZD';
const NETWORK = process.env.NETWORK || 'sepolia'; // mainnet, sepolia, goerli
const OUTPUT_DIR = process.env.OUTPUT_DIR || './';
const DELAY_MS = 200; // Rate limiting: 5 requests per second

// Network endpoints (V2 API - Unified)
const ETHERSCAN_V2_BASE_URL = 'api.etherscan.io';

// Chain IDs for V2 API
const CHAIN_IDS = {
    mainnet: 1,
    sepolia: 11155111,
    goerli: 5  // Note: Goerli might be deprecated
};

// CSV Headers
const CSV_HEADERS = [
    'hash',
    'blockNumber',
    'timeStamp',
    'from',
    'to',
    'value',
    'gas',
    'gasPrice',
    'gasUsed',
    'txnFee',
    'status',
    'isError',
    'input',
    'contractAddress',
    'cumulativeGasUsed',
    'confirmations'
].join(',');

class EtherscanClient {
    constructor(apiKey, network = 'sepolia') {
        this.apiKey = apiKey;
        this.network = network;
        this.baseUrl = ETHERSCAN_V2_BASE_URL;
        this.chainId = CHAIN_IDS[network] || CHAIN_IDS.sepolia;
        console.log(`Using ${network} network (Chain ID: ${this.chainId}) - V2 API: ${this.baseUrl}`);
    }

    // Test API connectivity
    async testConnection(address) {
        console.log('Testing API connectivity...');
        try {
            const balance = await this.makeRequest('account', 'balance', {
                address: address,
                tag: 'latest'
            });
            
            console.log(`✅ API connection successful. Balance: ${balance} wei`);
            return true;
        } catch (error) {
            console.log(`❌ API connection failed: ${error.message}`);
            return false;
        }
    }

    async makeRequest(module, action, params = {}) {
        return new Promise((resolve, reject) => {
            // V2 API format with chainid
            const queryParams = new URLSearchParams({
                chainid: this.chainId,
                module,
                action,
                apikey: this.apiKey,
                ...params
            });

            const fullUrl = `https://${this.baseUrl}/v2/api?${queryParams}`;
            console.log(`Making V2 API request: ${fullUrl}`);

            const options = {
                hostname: this.baseUrl,
                port: 443,
                path: `/v2/api?${queryParams}`,
                method: 'GET',
                headers: {
                    'User-Agent': 'go-tps-transaction-exporter/1.0'
                }
            };

            let data = '';

            const req = https.request(options, (res) => {
                res.on('data', (chunk) => {
                    data += chunk;
                });

                res.on('end', () => {
                    try {
                        const response = JSON.parse(data);
                        
                        // V2 API: status '1' means success, status '0' means error
                        if (response.status === '1') {
                            resolve(response.result);
                        } else if (response.status === '0') {
                            const errorMsg = response.message || response.result || 'Unknown error';
                            
                            // For some endpoints, empty results return NOTOK - treat as empty result
                            if (response.result === 'NOTOK' || errorMsg === 'No transactions found') {
                                if (action === 'txlist' || action === 'txlistinternal') {
                                    resolve([]); // Return empty array for transaction lists
                                } else if (action === 'balance') {
                                    resolve('0'); // Return zero balance
                                } else {
                                    resolve(null);
                                }
                            } else {
                                console.log('API Error Response:', JSON.stringify(response, null, 2));
                                reject(new Error(`API Error: ${errorMsg}`));
                            }
                        } else {
                            console.log('Unexpected API Response:', JSON.stringify(response, null, 2));
                            reject(new Error(`Unexpected API response format`));
                        }
                    } catch (error) {
                        console.log('Raw API Response:', data);
                        reject(new Error(`Parse Error: ${error.message}`));
                    }
                });
            });

            req.on('error', (error) => {
                reject(new Error(`Request Error: ${error.message}`));
            });

            req.end();
        });
    }

    async getAllTransactions(address, startBlock = 0, endBlock = 'latest') {
        console.log(`Fetching transactions for address: ${address}`);
        
        // Test API connectivity first
        const connectionOk = await this.testConnection(address);
        if (!connectionOk) {
            throw new Error('Failed to connect to Etherscan API. Please check your API key and network.');
        }
        
        const allTransactions = [];
        let page = 1;
        let hasMore = true;

        while (hasMore) {
            console.log(`Fetching page ${page}...`);
            
            try {
                const transactions = await this.makeRequest('account', 'txlist', {
                    address: address,
                    startblock: startBlock,
                    endblock: endBlock,
                    page: page,
                    offset: 10000,
                    sort: 'desc'
                });

                if (transactions && transactions.length > 0) {
                    allTransactions.push(...transactions);
                    console.log(`Retrieved ${transactions.length} transactions from page ${page}`);
                    
                    // If we got less than 10000 transactions, we've reached the end
                    if (transactions.length < 10000) {
                        hasMore = false;
                    } else {
                        page++;
                        // Rate limiting
                        await this.delay(DELAY_MS);
                    }
                } else {
                    hasMore = false;
                }
            } catch (error) {
                console.error(`Error fetching page ${page}:`, error.message);
                hasMore = false;
            }
        }

        console.log(`Total transactions retrieved: ${allTransactions.length}`);
        return allTransactions;
    }

    async getInternalTransactions(address, startBlock = 0, endBlock = 'latest') {
        console.log(`Fetching internal transactions for address: ${address}`);
        
        try {
            const transactions = await this.makeRequest('account', 'txlistinternal', {
                address: address,
                startblock: startBlock,
                endblock: endBlock,
                sort: 'desc'
            });

            console.log(`Retrieved ${transactions ? transactions.length : 0} internal transactions`);
            return transactions || [];
        } catch (error) {
            console.error('Error fetching internal transactions:', error.message);
            return [];
        }
    }

    delay(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }
}

function calculateTxnFee(gasUsed, gasPrice) {
    if (!gasUsed || !gasPrice) return '0';
    
    // Convert to BigInt to handle large numbers
    const fee = BigInt(gasUsed) * BigInt(gasPrice);
    return fee.toString();
}

function formatTransactionForCSV(transaction) {
    // Calculate transaction fee
    const txnFee = calculateTxnFee(transaction.gasUsed, transaction.gasPrice);
    
    // Escape commas and quotes in data fields
    const escapeCSV = (field) => {
        if (field === null || field === undefined) return '';
        const str = String(field);
        if (str.includes(',') || str.includes('"') || str.includes('\n')) {
            return `"${str.replace(/"/g, '""')}"`;
        }
        return str;
    };

    return [
        escapeCSV(transaction.hash),
        escapeCSV(transaction.blockNumber),
        escapeCSV(new Date(parseInt(transaction.timeStamp) * 1000).toISOString()),
        escapeCSV(transaction.from),
        escapeCSV(transaction.to),
        escapeCSV(transaction.value),
        escapeCSV(transaction.gas),
        escapeCSV(transaction.gasPrice),
        escapeCSV(transaction.gasUsed || '0'),
        escapeCSV(txnFee),
        escapeCSV(transaction.isError === '0' ? 'success' : 'failed'),
        escapeCSV(transaction.isError || '0'),
        escapeCSV(transaction.input ? transaction.input.substring(0, 100) + '...' : ''), // Truncate input data
        escapeCSV(transaction.contractAddress || ''),
        escapeCSV(transaction.cumulativeGasUsed || '0'),
        escapeCSV(transaction.confirmations || '0')
    ].join(',');
}

function formatTimestamp(timestamp) {
    return new Date(parseInt(timestamp) * 1000).toISOString().replace(/[:.]/g, '-');
}

function generateFilename(address, type = 'normal') {
    const timestamp = formatTimestamp(Date.now() / 1000);
    const shortAddress = address.substring(0, 8);
    return `${shortAddress}_${type}_transactions_${timestamp}.csv`;
}

async function exportTransactionsToCSV(address, includeInternal = false) {
    try {
        if (!address || !address.match(/^0x[a-fA-F0-9]{40}$/)) {
            throw new Error('Invalid Ethereum address format');
        }

        const client = new EtherscanClient(ETHERSCAN_API_KEY, NETWORK);
        
        // Fetch normal transactions
        const normalTransactions = await client.getAllTransactions(address);
        
        let allTransactions = [...normalTransactions];
        
        // Fetch internal transactions if requested
        if (includeInternal) {
            await client.delay(DELAY_MS);
            const internalTransactions = await client.getInternalTransactions(address);
            
            // Mark internal transactions
            const markedInternalTxs = internalTransactions.map(tx => ({
                ...tx,
                hash: tx.hash + '_internal',
                isInternal: true
            }));
            
            allTransactions = [...allTransactions, ...markedInternalTxs];
        }

        if (allTransactions.length === 0) {
            console.log('No transactions found for this address');
            return;
        }

        // Sort by timestamp (newest first)
        allTransactions.sort((a, b) => parseInt(b.timeStamp) - parseInt(a.timeStamp));

        // Generate filename
        const filename = generateFilename(address, includeInternal ? 'all' : 'normal');
        const filepath = path.join(OUTPUT_DIR, filename);

        // Create CSV content
        const csvLines = [CSV_HEADERS];
        allTransactions.forEach(tx => {
            csvLines.push(formatTransactionForCSV(tx));
        });

        // Write to file
        fs.writeFileSync(filepath, csvLines.join('\n'));
        
        console.log(`\n✅ Successfully exported ${allTransactions.length} transactions to: ${filepath}`);
        
        // Display summary
        const totalValue = allTransactions.reduce((sum, tx) => {
            return sum + BigInt(tx.value || '0');
        }, BigInt(0));
        
        const totalFees = allTransactions.reduce((sum, tx) => {
            const fee = calculateTxnFee(tx.gasUsed, tx.gasPrice);
            return sum + BigInt(fee || '0');
        }, BigInt(0));

        console.log('\n📊 Transaction Summary:');
        console.log(`   Address: ${address}`);
        console.log(`   Total Transactions: ${allTransactions.length}`);
        console.log(`   Normal Transactions: ${normalTransactions.length}`);
        if (includeInternal) {
            console.log(`   Internal Transactions: ${allTransactions.length - normalTransactions.length}`);
        }
        console.log(`   Total Value: ${totalValue.toString()} wei (${(Number(totalValue) / 1e18).toFixed(6)} ETH)`);
        console.log(`   Total Fees: ${totalFees.toString()} wei (${(Number(totalFees) / 1e18).toFixed(6)} ETH)`);

        if (allTransactions.length > 0) {
            const oldestTx = allTransactions[allTransactions.length - 1];
            const newestTx = allTransactions[0];
            console.log(`   Date Range: ${new Date(parseInt(oldestTx.timeStamp) * 1000).toLocaleDateString()} to ${new Date(parseInt(newestTx.timeStamp) * 1000).toLocaleDateString()}`);
        }

    } catch (error) {
        console.error('❌ Error:', error.message);
        process.exit(1);
    }
}

// Main execution
async function main() {
    console.log('🔍 Ethereum Address Transaction Exporter');
    console.log('=====================================\n');

    // Parse command line arguments
    const args = process.argv.slice(2);
    
    if (args.includes('--help') || args.includes('-h')) {
        console.log('Usage: node export-address-transactions.js [OPTIONS] <address>');
        console.log('\nOptions:');
        console.log('  --include-internal    Include internal transactions');
        console.log('  --help, -h           Show this help message');
        console.log('\nEnvironment Variables:');
        console.log('  ETHERSCAN_API_KEY    Your Etherscan API key (required)');
        console.log('  NETWORK              Network to use: mainnet, sepolia, goerli (default: sepolia)');
        console.log('  OUTPUT_DIR           Output directory (default: current directory)');
        console.log('\nExample:');
        console.log('  ETHERSCAN_API_KEY=your_key node export-address-transactions.js 0x742d35cc6460c0dbc25b35b5c65d5ebaeacadc21');
        console.log('  ETHERSCAN_API_KEY=your_key NETWORK=mainnet node export-address-transactions.js 0x742d35cc6460c0dbc25b35b5c65d5ebaeacadc21');
        console.log('  ETHERSCAN_API_KEY=your_key node export-address-transactions.js --include-internal 0x742d35cc6460c0dbc25b35b5c65d5ebaeacadc21');
        return;
    }

    const includeInternal = args.includes('--include-internal');
    const address = args.find(arg => arg.startsWith('0x'));

    if (!address) {
        console.error('❌ Error: Please provide an Ethereum address');
        console.log('Use --help for usage information');
        process.exit(1);
    }

    if (ETHERSCAN_API_KEY === 'YourApiKeyToken') {
        console.error('❌ Error: Please set your Etherscan API key');
        console.log('Set the ETHERSCAN_API_KEY environment variable');
        console.log('Example: ETHERSCAN_API_KEY=your_key node export-address-transactions.js ' + address);
        process.exit(1);
    }

    await exportTransactionsToCSV(address, includeInternal);
}

// Handle uncaught exceptions
process.on('uncaughtException', (error) => {
    console.error('❌ Uncaught Exception:', error.message);
    process.exit(1);
});

process.on('unhandledRejection', (error) => {
    console.error('❌ Unhandled Rejection:', error.message);
    process.exit(1);
});

// Run the script
if (require.main === module) {
    main();
}

module.exports = { EtherscanClient, exportTransactionsToCSV };