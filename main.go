package main

import (
	"bufio"
	"context"
	"fmt"
	"math/big"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/joho/godotenv"
)

const (
	DefaultRPCURL             = "http://localhost:8545"
	DefaultWSURL              = "" // Empty = no WebSocket, will use RPC polling
	DefaultDBPath             = "./transactions.db"
	DefaultWalletCount        = 10
	DefaultTxPerWallet        = 10
	DefaultValueWei           = "1000000000000000" // 0.001 ETH
	DefaultToAddress          = "0x0000000000000000000000000000000000000001"
	DefaultRunDurationMinutes = 0  // 0 = run once, >0 = loop for duration
	DefaultReceiptWorkers     = 10 // Number of concurrent workers for receipt confirmation
)

type Config struct {
	RPCURL             string
	WSURL              string
	DBPath             string
	Mnemonic           string
	WalletCount        int
	TxPerWallet        int
	ValueWei           string
	ToAddress          string
	RunDurationMinutes int
	ReceiptWorkers     int
}

// ReceiptJob represents a receipt confirmation job
type ReceiptJob struct {
	DBPath    string
	RPCURL    string
	WSClient  *ethclient.Client
	TxHash    string
	Nonce     uint64
	StartTime time.Time
	WalletNum int
}

func main() {
	fmt.Println("=== Ethereum TPS Tester ===")
	fmt.Println()

	// Load .env file if it exists (optional)
	if err := godotenv.Load(); err != nil {
		fmt.Println("No .env file found, using environment variables or defaults")
	}

	// Load configuration
	config := LoadConfig()

	// Initialize database
	fmt.Println("Initializing database...")
	db, err := NewDatabase(config.DBPath)
	if err != nil {
		fmt.Printf("Error initializing database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	fmt.Println("✓ Database initialized")

	// Connect to RPC
	fmt.Printf("Connecting to RPC: %s\n", config.RPCURL)
	txSender, err := NewTransactionSender(config.RPCURL)
	if err != nil {
		fmt.Printf("Error connecting to RPC: %v\n", err)
		os.Exit(1)
	}
	defer txSender.Close()
	fmt.Println("✓ Connected to RPC")

	// Connect to WebSocket if URL is provided (for faster receipt confirmations)
	var wsClient *ethclient.Client
	if config.WSURL != "" {
		fmt.Printf("Connecting to WebSocket: %s\n", config.WSURL)
		wsClient, err = ethclient.Dial(config.WSURL)
		if err != nil {
			fmt.Printf("Warning: Could not connect to WebSocket (will use RPC polling): %v\n", err)
			wsClient = nil
		} else {
			defer wsClient.Close()
			fmt.Println("✓ Connected to WebSocket")
		}
	} else {
		fmt.Println("No WebSocket URL provided, will use RPC polling for receipts")
	}

	// Get or generate mnemonic
	var mnemonic string
	if config.Mnemonic != "" {
		fmt.Println("\nUsing provided mnemonic...")
		mnemonic = config.Mnemonic
	} else {
		fmt.Println("\nGenerating new mnemonic...")
		var err error
		mnemonic, err = GenerateMnemonic()
		if err != nil {
			fmt.Printf("Error generating mnemonic: %v\n", err)
			os.Exit(1)
		}
	}

	// Generate wallets from single mnemonic
	fmt.Printf("Deriving %d wallets from mnemonic...\n", config.WalletCount)

	wallets, err := DeriveWalletsFromMnemonic(mnemonic, config.WalletCount)
	if err != nil {
		fmt.Printf("Error deriving wallets: %v\n", err)
		os.Exit(1)
	}

	// Save mnemonic to file
	err = SaveMnemonicToFile("mnemonic.txt", mnemonic)
	if err != nil {
		fmt.Printf("Warning: Could not save mnemonic: %v\n", err)
	}

	fmt.Printf("✓ Generated %d wallets\n", len(wallets))

	// Save wallets to database
	fmt.Println("\nSaving wallets to database...")
	for _, wallet := range wallets {
		err := db.InsertWallet(wallet.Address.Hex(), wallet.DerivationPath)
		if err != nil {
			fmt.Printf("Warning: Could not save wallet %s: %v\n", wallet.Address.Hex(), err)
		}
	}
	fmt.Println("✓ Wallets saved to database")

	// Display wallet addresses and balances
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("WALLET ADDRESSES AND BALANCES")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	ctx := context.Background()
	allFunded := true

	for i, wallet := range wallets {
		balance, err := txSender.GetBalance(ctx, wallet.Address)
		if err != nil {
			fmt.Printf("[%d] %s\n", i+1, wallet.Address.Hex())
			fmt.Printf("    Balance: ERROR - %v\n", err)
			allFunded = false
		} else {
			// Convert balance to ETH for display
			balanceFloat := new(big.Float).SetInt(balance)
			ethValue := new(big.Float).Quo(balanceFloat, big.NewFloat(1e18))

			fmt.Printf("[%d] %s\n", i+1, wallet.Address.Hex())
			fmt.Printf("    Balance: %s wei (%.6f ETH)\n", balance.String(), ethValue)

			// Check if balance is zero
			if balance.Cmp(big.NewInt(0)) == 0 {
				fmt.Printf("    ⚠️  WARNING: Wallet has ZERO balance!\n")
				allFunded = false
			}
		}
		fmt.Println()
	}

	fmt.Println(strings.Repeat("=", 60))
	if !allFunded {
		fmt.Println("⚠️  WARNING: Some wallets have zero balance or errors!")
	}
	fmt.Println()

	// Ask for user confirmation (only once)
	fmt.Print("Do you want to proceed with sending transactions? (y/n): ")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	response := strings.TrimSpace(strings.ToLower(scanner.Text()))

	if response != "y" && response != "yes" {
		fmt.Println("\nOperation cancelled by user.")
		fmt.Println("Please fund the wallets and try again.")
		os.Exit(0)
	}

	fmt.Println("\n✓ User confirmed. Proceeding with transactions...")
	fmt.Println()

	// Check if we should run in loop mode
	if config.RunDurationMinutes > 0 {
		fmt.Printf("Running in LOOP MODE for %d minutes\n", config.RunDurationMinutes)
		fmt.Println()
		runInLoopMode(config, db, txSender, wsClient, wallets)
	} else {
		fmt.Println("Running in SINGLE MODE")
		fmt.Println()

		// Record start time for single execution
		executionStart := time.Now()

		runSingleExecution(config, db, txSender, wsClient, wallets)

		// Calculate elapsed time and ensure minimum 1 second
		executionElapsed := time.Since(executionStart)
		minDuration := 1 * time.Second

		if executionElapsed < minDuration {
			remainingSleep := minDuration - executionElapsed
			fmt.Printf("\n⏱  Execution completed in %.6f seconds. Waiting %.6f seconds to maintain 1-second minimum...\n",
				executionElapsed.Seconds(), remainingSleep.Seconds())
			time.Sleep(remainingSleep)
		} else {
			fmt.Printf("\n⏱  Execution completed in %.6f seconds\n", executionElapsed.Seconds())
		}
	}

	// sleep for 10 seconds before creating summary to allow any pending receipt confirmations to finish
	fmt.Println("\nWaiting a few seconds for any pending receipt confirmations to finish...")
	time.Sleep(60 * time.Second)

	// Final summary
	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("✓ All executions completed")
	fmt.Printf("✓ Mnemonic saved to: mnemonic.txt\n")
	fmt.Printf("✓ Database: %s\n", config.DBPath)
	fmt.Println(strings.Repeat("=", 60))
}

func runInLoopMode(config *Config, db *Database, txSender *TransactionSender, wsClient *ethclient.Client, wallets []*Wallet) {
	duration := time.Duration(config.RunDurationMinutes) * time.Minute
	startTime := time.Now()
	endTime := startTime.Add(duration)
	iteration := 0

	fmt.Printf("Loop started at: %s\n", startTime.Format("15:04:05"))
	fmt.Printf("Will run until: %s\n", endTime.Format("15:04:05"))
	fmt.Println(strings.Repeat("=", 60))

	for time.Now().Before(endTime) {
		iteration++
		remainingTime := time.Until(endTime)
		fmt.Printf("\n\n[ITERATION #%d] Time remaining: %.1f minutes\n", iteration, remainingTime.Minutes())
		fmt.Println(strings.Repeat("-", 60))

		// Record start time for this iteration
		iterationStart := time.Now()

		runSingleExecution(config, db, txSender, wsClient, wallets)

		// Calculate elapsed time and ensure minimum 1 second per iteration
		iterationElapsed := time.Since(iterationStart)
		minDuration := 1 * time.Second

		if iterationElapsed < minDuration {
			remainingSleep := minDuration - iterationElapsed
			fmt.Printf("\n⏱  Iteration completed in %.3f seconds. Waiting %.3f seconds to maintain 1-second minimum...\n",
				iterationElapsed.Seconds(), remainingSleep.Seconds())
			time.Sleep(remainingSleep)
		} else {
			fmt.Printf("\n⏱  Iteration completed in %.3f seconds\n", iterationElapsed.Seconds())
		}
	}

	totalDuration := time.Since(startTime)
	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("=== LOOP MODE COMPLETED ===")
	fmt.Println()
	fmt.Printf("Total iterations: %d\n", iteration)
	fmt.Printf("Total duration: %.2f minutes\n", totalDuration.Minutes())
	fmt.Println(strings.Repeat("=", 60))
}

func runSingleExecution(config *Config, db *Database, txSender *TransactionSender, wsClient *ethclient.Client, wallets []*Wallet) {
	// Generate unique batch number for this execution
	batchNumber := fmt.Sprintf("batch-%s", time.Now().Format("20060102-150405"))
	fmt.Printf("Batch Number: %s\n\n", batchNumber)

	ctx := context.Background()

	// Create receipt worker pool
	receiptJobChan := make(chan ReceiptJob, config.WalletCount*config.TxPerWallet)
	var receiptWG sync.WaitGroup
	startReceiptWorkerPool(config.ReceiptWorkers, receiptJobChan, &receiptWG)
	fmt.Printf("📋 Started %d receipt confirmation workers\n\n", config.ReceiptWorkers)

	// Create database writer channel for serialized writes
	dbWriteChan := make(chan *Transaction, config.WalletCount*config.TxPerWallet)
	var dbWriterWG sync.WaitGroup
	dbWriterWG.Add(1)
	go func() {
		defer dbWriterWG.Done()
		for tx := range dbWriteChan {
			_, err := db.InsertTransaction(tx)
			if err != nil {
				fmt.Printf("  Warning: Could not save transaction to DB: %v\n", err)
			}
		}
	}()

	// Parse configuration values
	value := new(big.Int)
	value.SetString(config.ValueWei, 10)
	toAddress := common.HexToAddress(config.ToAddress)

	fmt.Printf("\nTransaction Configuration:\n")
	fmt.Printf("  - Number of wallets: %d\n", len(wallets))
	fmt.Printf("  - Transactions per wallet: %d\n", config.TxPerWallet)
	fmt.Printf("  - Total transactions: %d\n", len(wallets)*config.TxPerWallet)
	fmt.Printf("  - Target address: %s\n", toAddress.Hex())
	fmt.Printf("  - Value per tx: %s wei\n", value.String())
	fmt.Println()

	// Create and send transactions
	totalTransactions := 0
	totalSuccessful := 0
	totalFailed := 0
	startTime := time.Now()

	// Use mutex for thread-safe counter updates
	var mu sync.Mutex
	var wgSubmit sync.WaitGroup // Wait for transaction submissions only
	// Receipt confirmations happen in background, we don't wait for them

	fmt.Println("Starting transaction submission...")
	fmt.Println(strings.Repeat("=", 60))

	// Process all wallets in parallel
	for walletIdx, wallet := range wallets {
		wgSubmit.Add(1)
		go func(idx int, w *Wallet) {
			defer wgSubmit.Done()

			fmt.Printf("\n[Wallet %d/%d] (%s)\n",
				idx+1, len(wallets), w.Address.Hex())

			// Prepare batch transactions with precalculated nonces
			txRequests, err := txSender.PrepareBatchTransactions(
				ctx,
				w,
				toAddress,
				value,
				config.TxPerWallet,
			)

			if err != nil {
				fmt.Printf("  Error preparing transactions: %v\n", err)
				return
			}

			// Send all transactions for this wallet
			for txIdx, req := range txRequests {
				result, err := txSender.CreateAndSendTransaction(ctx, req)

				// Create database transaction record
				dbTx := &Transaction{
					BatchNumber:   batchNumber,
					WalletAddress: w.Address.Hex(),
					Nonce:         req.Nonce,
					ToAddress:     toAddress.Hex(),
					Value:         value.String(),
					GasPrice:      req.GasPrice.String(),
					GasLimit:      req.GasLimit,
					SubmittedAt:   result.SubmittedAt,
					ExecutionTime: result.ExecutionTime,
				}

				if err != nil {
					dbTx.Status = "failed"
					dbTx.Error = err.Error()
					mu.Lock()
					totalFailed++
					totalTransactions++
					mu.Unlock()

					// Send to database writer (non-blocking)
					dbWriteChan <- dbTx
				} else {
					dbTx.TxHash = result.TxHash
					dbTx.Status = "pending"

					// Send to database writer (non-blocking)
					dbWriteChan <- dbTx

					fmt.Printf("  [W%d] Tx %d sent (nonce %d): %s\n", idx+1, txIdx+1, req.Nonce, result.TxHash[:16]+"...")

					mu.Lock()
					totalTransactions++
					mu.Unlock()

					// Send job to receipt worker pool (non-blocking)
					receiptJobChan <- ReceiptJob{
						DBPath:    config.DBPath,
						RPCURL:    config.RPCURL,
						WSClient:  wsClient,
						TxHash:    result.TxHash,
						Nonce:     req.Nonce,
						StartTime: result.SubmittedAt,
						WalletNum: idx + 1,
					}
				}
			}

			fmt.Printf("  [W%d] ✓ Sent %d transactions (nonce %d to %d)\n",
				idx+1,
				len(txRequests),
				txRequests[0].Nonce,
				txRequests[len(txRequests)-1].Nonce,
			)
		}(walletIdx, wallet)
	}

	// Launch background goroutine to wait for submissions and print summary (non-blocking)
	go func() {
		fmt.Println("\nWaiting for all transactions to be submitted...")
		wgSubmit.Wait()
		fmt.Println("✓ All transactions submitted")

		// Close the database writer channel and wait for all writes to complete
		close(dbWriteChan)
		dbWriterWG.Wait()
		fmt.Println("✓ All transactions saved to database")

		// Close the receipt job channel now that all jobs are submitted
		close(receiptJobChan)
		fmt.Println("Note: Receipt confirmations are happening in background")

		totalTime := time.Since(startTime)

		fmt.Println()
		fmt.Println(strings.Repeat("=", 60))
		fmt.Println("=== Execution Summary ===")
		fmt.Println()
		fmt.Printf("Batch Number: %s\n", batchNumber)

		// Lock to safely read counters
		mu.Lock()
		submitted := totalTransactions
		failed := totalFailed
		successful := totalSuccessful
		mu.Unlock()

		fmt.Printf("Total transactions submitted: %d\n", submitted)
		fmt.Printf("Successful: %d\n", successful)
		fmt.Printf("Failed: %d\n", failed)
		fmt.Printf("Total execution time: %.2f seconds\n", totalTime.Seconds())
		if submitted > 0 {
			fmt.Printf("Average time per transaction: %.2f ms\n",
				totalTime.Seconds()*1000/float64(submitted))
			fmt.Printf("Transactions per second: %.2f\n",
				float64(submitted)/totalTime.Seconds())
		}
		fmt.Println()

		// Get database statistics
		stats, err := db.GetTransactionStats()
		if err != nil {
			fmt.Printf("Warning: Could not get database stats: %v\n", err)
		} else {
			fmt.Println("=== Database Statistics ===")
			fmt.Println()
			for key, value := range stats {
				fmt.Printf("%s: %v\n", key, value)
			}
		}

		fmt.Println()
		fmt.Printf("✓ All data saved to database: %s\n", config.DBPath)
		fmt.Println()
		fmt.Println("Done!")
		fmt.Println("(Receipt confirmations continue in background)")
	}()

	// Return immediately - submissions and confirmations happen in background
	fmt.Println("\n✓ Transaction submission launched in background")
}

// startReceiptWorkerPool starts a pool of workers to process receipt confirmations
func startReceiptWorkerPool(workerCount int, jobChan <-chan ReceiptJob, wg *sync.WaitGroup) {
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go receiptWorker(i+1, jobChan, wg)
	}
}

// receiptWorker processes receipt confirmation jobs from the job channel
func receiptWorker(workerID int, jobChan <-chan ReceiptJob, wg *sync.WaitGroup) {
	defer wg.Done()

	var db *Database
	var txSender *TransactionSender
	var currentDBPath string
	var currentRPCURL string

	// Process jobs from channel
	for job := range jobChan {
		// Initialize or reuse connections based on job config
		if db == nil || currentDBPath != job.DBPath {
			if db != nil {
				db.Close()
			}
			var err error
			db, err = NewDatabase(job.DBPath)
			if err != nil {
				fmt.Printf("[Worker %d] Error: Could not open DB: %v\n", workerID, err)
				continue
			}
			currentDBPath = job.DBPath
		}

		if txSender == nil || currentRPCURL != job.RPCURL {
			if txSender != nil {
				txSender.Close()
			}
			var err error
			txSender, err = NewTransactionSender(job.RPCURL)
			if err != nil {
				fmt.Printf("[Worker %d] Error: Could not connect to RPC: %v\n", workerID, err)
				continue
			}
			currentRPCURL = job.RPCURL
		}

		processReceiptJob(workerID, db, txSender, job)
	}

	// Cleanup connections
	if db != nil {
		db.Close()
	}
	if txSender != nil {
		txSender.Close()
	}
}

// processReceiptJob processes a single receipt confirmation job
func processReceiptJob(workerID int, db *Database, txSender *TransactionSender, job ReceiptJob) {
	// Wait for receipt with timeout - use shared WebSocket if available
	ctx := context.Background()
	receipt, receiptErr := txSender.WaitForReceiptWithSharedWebSocket(ctx, job.WSClient, common.HexToHash(job.TxHash), 60*time.Second)

	// Update database with final status
	confirmedAt := time.Now()
	execTime := confirmedAt.Sub(job.StartTime).Seconds() * 1000

	if receiptErr != nil {
		db.UpdateTransactionStatus(job.TxHash, "failed", nil, execTime, receiptErr.Error())
		fmt.Printf("  [W%d] Tx (nonce %d): ✗ timeout/error\n", job.WalletNum, job.Nonce)
	} else {
		if receipt.Status == 1 {
			db.UpdateTransactionStatus(job.TxHash, "success", &confirmedAt, execTime, "")
			fmt.Printf("  [W%d] Tx (nonce %d): ✓ confirmed in %.2fs\n", job.WalletNum, job.Nonce, execTime/1000)
		} else {
			db.UpdateTransactionStatus(job.TxHash, "failed", &confirmedAt, execTime, "transaction reverted")
			fmt.Printf("  [W%d] Tx (nonce %d): ✗ reverted\n", job.WalletNum, job.Nonce)
		}
	}
}

// waitForReceiptInBackground waits for a transaction receipt in a completely independent goroutine
// DEPRECATED: Use worker pool pattern instead
// It creates its own database and RPC connections to avoid lifecycle issues
// Uses shared WebSocket client if available, otherwise falls back to RPC polling
func waitForReceiptInBackground(dbPath, rpcURL string, wsClient *ethclient.Client, txHash string, nonce uint64, startTime time.Time, walletNum int) {
	// Create independent database connection for this goroutine
	db, err := NewDatabase(dbPath)
	if err != nil {
		fmt.Printf("  [W%d] Warning: Could not open DB for receipt confirmation: %v\n", walletNum, err)
		return
	}
	defer db.Close()

	// Create independent RPC connection for fallback
	txSender, err := NewTransactionSender(rpcURL)
	if err != nil {
		fmt.Printf("  [W%d] Warning: Could not connect to RPC for receipt confirmation: %v\n", walletNum, err)
		return
	}
	defer txSender.Close()

	// Wait for receipt with timeout - use shared WebSocket if available
	ctx := context.Background()
	receipt, receiptErr := txSender.WaitForReceiptWithSharedWebSocket(ctx, wsClient, common.HexToHash(txHash), 60*time.Second)

	// Update database with final status
	confirmedAt := time.Now()
	execTime := confirmedAt.Sub(startTime).Seconds() * 1000

	if receiptErr != nil {
		db.UpdateTransactionStatus(txHash, "failed", nil, execTime, receiptErr.Error())
		fmt.Printf("  [W%d] Tx (nonce %d): ✗ timeout/error\n", walletNum, nonce)
	} else {
		if receipt.Status == 1 {
			db.UpdateTransactionStatus(txHash, "success", &confirmedAt, execTime, "")
			fmt.Printf("  [W%d] Tx (nonce %d): ✓ confirmed in %.2fs\n", walletNum, nonce, execTime/1000)
		} else {
			db.UpdateTransactionStatus(txHash, "failed", &confirmedAt, execTime, "transaction reverted")
			fmt.Printf("  [W%d] Tx (nonce %d): ✗ reverted\n", walletNum, nonce)
		}
	}
}

func LoadConfig() *Config {
	// Load from environment variables or use defaults
	config := &Config{
		RPCURL:             getEnv("RPC_URL", DefaultRPCURL),
		WSURL:              getEnv("WS_URL", DefaultWSURL),
		DBPath:             getEnv("DB_PATH", DefaultDBPath),
		Mnemonic:           getEnv("MNEMONIC", ""),
		WalletCount:        getEnvInt("WALLET_COUNT", DefaultWalletCount),
		TxPerWallet:        getEnvInt("TX_PER_WALLET", DefaultTxPerWallet),
		ValueWei:           getEnv("VALUE_WEI", DefaultValueWei),
		ToAddress:          getEnv("TO_ADDRESS", DefaultToAddress),
		RunDurationMinutes: getEnvInt("RUN_DURATION_MINUTES", DefaultRunDurationMinutes),
		ReceiptWorkers:     getEnvInt("RECEIPT_WORKERS", DefaultReceiptWorkers),
	}

	return config
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	var intValue int
	_, err := fmt.Sscanf(value, "%d", &intValue)
	if err != nil {
		fmt.Printf("Warning: Invalid integer value for %s: '%s', using default: %d\n", key, value, defaultValue)
		return defaultValue
	}
	return intValue
}

func SaveMnemonicToFile(filename string, mnemonic string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	file.WriteString("=== MNEMONIC PHRASE ===\n")
	file.WriteString("KEEP THIS SAFE AND PRIVATE!\n")
	file.WriteString("Generated: " + time.Now().Format(time.RFC3339) + "\n\n")
	file.WriteString(mnemonic + "\n")

	return nil
}
