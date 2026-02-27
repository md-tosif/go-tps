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
	DefaultRunDurationMinutes = 0       // 0 = run once, >0 = loop for duration
	DefaultReceiptWorkers     = 10      // Number of concurrent workers for receipt confirmation
	DefaultLogLevel           = "DEBUG" // DEBUG, INFO, WARN, ERROR
)

// LogLevel represents logging levels
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

// Global logging configuration
var currentLogLevel LogLevel = INFO

// parseLogLevel converts string to LogLevel
func parseLogLevel(level string) LogLevel {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return DEBUG
	case "INFO":
		return INFO
	case "WARN", "WARNING":
		return WARN
	case "ERROR":
		return ERROR
	default:
		return INFO
	}
}

// Logging functions
func logDebug(format string, args ...interface{}) {
	if currentLogLevel <= DEBUG {
		fmt.Printf("[DEBUG] "+format, args...)
	}
}

func logInfo(format string, args ...interface{}) {
	if currentLogLevel <= INFO {
		fmt.Printf("[INFO] "+format, args...)
	}
}

func logWarn(format string, args ...interface{}) {
	if currentLogLevel <= WARN {
		fmt.Printf("[WARN] "+format, args...)
	}
}

func logError(format string, args ...interface{}) {
	if currentLogLevel <= ERROR {
		fmt.Printf("[ERROR] "+format, args...)
	}
}

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
	LogLevel           string
}

// ReceiptJob represents a receipt confirmation job
type ReceiptJob struct {
	DB        *Database
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
		logDebug("No .env file found, using environment variables or defaults\n")
	}

	// Load configuration
	config := LoadConfig()

	// Initialize database
	logInfo("Initializing database...\n")
	db, err := NewDatabase(config.DBPath)
	if err != nil {
		logError("Error initializing database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	logInfo("✓ Database initialized\n")

	// Connect to RPC
	logInfo("Connecting to RPC: %s\n", config.RPCURL)
	txSender, err := NewTransactionSender(config.RPCURL)
	if err != nil {
		logError("Error connecting to RPC: %v\n", err)
		os.Exit(1)
	}
	defer txSender.Close()
	logInfo("✓ Connected to RPC\n")

	// Connect to WebSocket if URL is provided (for faster receipt confirmations)
	var wsClient *ethclient.Client
	if config.WSURL != "" {
		logInfo("Connecting to WebSocket: %s\n", config.WSURL)
		wsClient, err = ethclient.Dial(config.WSURL)
		if err != nil {
			logWarn("Could not connect to WebSocket (will use RPC polling): %v\n", err)
			wsClient = nil
		} else {
			defer wsClient.Close()
			logInfo("✓ Connected to WebSocket\n")
		}
	} else {
		logDebug("No WebSocket URL provided, will use RPC polling for receipts\n")
	}

	// Get or generate mnemonic
	var mnemonic string
	if config.Mnemonic != "" {
		logInfo("\nUsing provided mnemonic...\n")
		mnemonic = config.Mnemonic
	} else {
		logInfo("\nGenerating new mnemonic...\n")
		var err error
		mnemonic, err = GenerateMnemonic()
		if err != nil {
			logError("Error generating mnemonic: %v\n", err)
			os.Exit(1)
		}
	}

	// Generate wallets from single mnemonic
	logInfo("Deriving %d wallets from mnemonic...\n", config.WalletCount)

	wallets, err := DeriveWalletsFromMnemonic(mnemonic, config.WalletCount)
	if err != nil {
		logError("Error deriving wallets: %v\n", err)
		os.Exit(1)
	}

	// Save mnemonic to file
	err = SaveMnemonicToFile("mnemonic.txt", mnemonic)
	if err != nil {
		logWarn("Could not save mnemonic: %v\n", err)
	}

	logInfo("✓ Generated %d wallets\n", len(wallets))

	// Save wallets to database
	logInfo("\nSaving wallets to database...\n")
	for _, wallet := range wallets {
		err := db.InsertWallet(wallet.Address.Hex(), wallet.DerivationPath)
		if err != nil {
			logWarn("Could not save wallet %s: %v\n", wallet.Address.Hex(), err)
		}
	}
	logInfo("✓ Wallets saved to database\n")

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
			logDebug("[%d] %s\n", i+1, wallet.Address.Hex())
			logError("    Balance: ERROR - %v\n", err)
			allFunded = false
		} else {
			// Convert balance to ETH for display
			balanceFloat := new(big.Float).SetInt(balance)
			ethValue := new(big.Float).Quo(balanceFloat, big.NewFloat(1e18))

			fmt.Printf("[%d] %s\n", i+1, wallet.Address.Hex())
			fmt.Printf("    Balance: %s wei (%.6f ETH)\n", balance.String(), ethValue)

			// Check if balance is zero
			if balance.Cmp(big.NewInt(0)) == 0 {
				logWarn("    ⚠️  WARNING: Wallet has ZERO balance!\n")
				allFunded = false
			}
		}
		logDebug("\n")
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
	logInfo("📋 Started %d receipt confirmation workers\n\n", config.ReceiptWorkers)

	// Parse configuration values
	value := new(big.Int)
	value.SetString(config.ValueWei, 10)
	toAddress := common.HexToAddress(config.ToAddress)

	logInfo("\nTransaction Configuration:\n")
	logInfo("  - Number of wallets: %d\n", len(wallets))
	logInfo("  - Transactions per wallet: %d\n", config.TxPerWallet)
	logInfo("  - Total transactions: %d\n", len(wallets)*config.TxPerWallet)
	logInfo("  - Target address: %s\n", toAddress.Hex())
	logInfo("  - Value per tx: %s wei\n", value.String())
	logInfo("\n")

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

			logDebug("\n[Wallet %d/%d] (%s)\n",
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
				logError("  Error preparing transactions: %v\n", err)
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

					// Print failure reason
					logError("  [W%d] Tx %d FAILED (nonce %d): %v\n", idx+1, txIdx+1, req.Nonce, err)

					// Save to database (mutex protected)
					if _, err := db.InsertTransaction(dbTx); err != nil {
						logWarn("  Could not save failed transaction to DB: %v\n", err)
					}
				} else {
					dbTx.TxHash = result.TxHash
					dbTx.Status = "pending"

					// Save to database (mutex protected)
					if _, err := db.InsertTransaction(dbTx); err != nil {
						logWarn("  Could not save transaction to DB: %v\n", err)
					}

					logDebug("  [W%d] Tx %d sent (nonce %d): %s\n", idx+1, txIdx+1, req.Nonce, result.TxHash[:16]+"...")

					mu.Lock()
					totalTransactions++
					mu.Unlock()

					// Send job to receipt worker pool (non-blocking)
					receiptJobChan <- ReceiptJob{
						DB:        db,
						RPCURL:    config.RPCURL,
						WSClient:  wsClient,
						TxHash:    result.TxHash,
						Nonce:     req.Nonce,
						StartTime: result.SubmittedAt,
						WalletNum: idx + 1,
					}
				}
			}

			logInfo("  [W%d] ✓ Sent %d transactions (nonce %d to %d)\n",
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

		// Display failed transactions if any
		if failed > 0 {
			fmt.Println("=== Failed Transactions ===")
			fmt.Println()

			// Query failed transactions from database
			failures, err := db.GetFailedTransactions(batchNumber, 20)

			if err == nil && len(failures) > 0 {
				for i, fail := range failures {
					walletShort := fail["wallet_address"]
					if len(walletShort) > 10 {
						walletShort = walletShort[:10] + "..."
					}
					fmt.Printf("  %d. Wallet %s (nonce %s): %s\n",
						i+1, walletShort, fail["nonce"], fail["error"])
				}
				if failed > 20 {
					fmt.Printf("  ... and %d more (showing first 20)\n", failed-20)
				}
			} else if err != nil {
				fmt.Printf("  Could not retrieve failed transactions: %v\n", err)
			} else if len(failures) == 0 && failed > 0 {
				fmt.Println("  (Failed transactions not yet recorded in database)")
			}
			fmt.Println()
		}

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

	var txSender *TransactionSender
	var currentRPCURL string

	// Process jobs from channel
	for job := range jobChan {
		// Initialize or reuse RPC connection
		if txSender == nil || currentRPCURL != job.RPCURL {
			if txSender != nil {
				txSender.Close()
			}
			var err error
			txSender, err = NewTransactionSender(job.RPCURL)
			if err != nil {
				logError("[Worker %d] Could not connect to RPC: %v\n", workerID, err)
				continue
			}
			currentRPCURL = job.RPCURL
		}

		processReceiptJob(workerID, txSender, job)
	}

	// Cleanup RPC connection
	if txSender != nil {
		txSender.Close()
	}
}

// processReceiptJob processes a single receipt confirmation job
func processReceiptJob(workerID int, txSender *TransactionSender, job ReceiptJob) {
	// Wait for receipt with timeout - use shared WebSocket if available
	ctx := context.Background()
	receipt, receiptErr := txSender.WaitForReceiptWithSharedWebSocket(ctx, job.WSClient, common.HexToHash(job.TxHash), 60*time.Second)

	// Update database with final status
	confirmedAt := time.Now()
	execTime := confirmedAt.Sub(job.StartTime).Seconds() * 1000

	if receiptErr != nil {
		job.DB.UpdateTransactionStatus(job.TxHash, "failed", nil, execTime, receiptErr.Error())
		logWarn("  [W%d] Tx (nonce %d): ✗ timeout/error - %v\n", job.WalletNum, job.Nonce, receiptErr)
	} else {
		if receipt.Status == 1 {
			job.DB.UpdateTransactionStatus(job.TxHash, "success", &confirmedAt, execTime, "")
			logInfo("  [W%d] Tx (nonce %d): ✓ confirmed in %.2fs\n", job.WalletNum, job.Nonce, execTime/1000)
		} else {
			job.DB.UpdateTransactionStatus(job.TxHash, "failed", &confirmedAt, execTime, "transaction reverted")
			logWarn("  [W%d] Tx (nonce %d): ✗ reverted (transaction failed on-chain)\n", job.WalletNum, job.Nonce)
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
		LogLevel:           getEnv("LOG_LEVEL", DefaultLogLevel),
	}

	// Set global log level
	currentLogLevel = parseLogLevel(config.LogLevel)

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
