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

	dbpkg "go-tps/db"
	"go-tps/logger"
	txpkg "go-tps/tx"
	"go-tps/wallet"
	"go-tps/worker"

	"github.com/ethereum/go-ethereum/common"
	"github.com/joho/godotenv"
)

const (
	DefaultRPCURL             = "http://localhost:8545"
	DefaultWSURL              = "http://localhost:8546" // Empty = no WebSocket, will use RPC polling
	DefaultDBPath             = "./transactions.db"
	DefaultWalletCount        = 10
	DefaultTxPerWallet        = 10
	DefaultValueWei           = "1000000000000000" // 0.001 ETH
	DefaultToAddress          = "0x0000000000000000000000000000000000000001"
	DefaultRunDurationMinutes = 0       // 0 = run once, >0 = loop for duration
	DefaultReceiptWorkers     = 4       // Tuned for small machines; override with RECEIPT_WORKERS
	DefaultLogLevel           = "DEBUG" // DEBUG, INFO, WARN, ERROR
	DefaultAutomatedMode      = false   // true = skip user confirmation
	DefaultContextTimeout     = 30      // seconds for RPC calls
	DefaultDBRetentionDays    = 30      // cleanup records older than this
	DefaultWSReconnectDelay   = 5       // seconds before reconnecting WebSocket
	DefaultBufferSize         = 500     // channel buffer size (0 = auto-calculate from WalletCount * TxPerWallet)
	DefaultDBMaxOpenConns     = 25      // max open DB connections
	DefaultDBMaxIdleConns     = 2       // max idle DB connections
)

// (logging implementation moved to the logger package)

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
	AutomatedMode      bool // Skip user confirmation if true
	ContextTimeout     int  // Timeout for RPC calls in seconds
	DBRetentionDays    int  // Cleanup records older than this
	WSReconnectDelay   int  // Seconds before reconnecting WebSocket
	BufferSize         int  // Channel buffer size (0 = auto-calculate)
	DBMaxOpenConns     int  // Max open SQLite connections
	DBMaxIdleConns     int  // Max idle SQLite connections
}

func main() {
	fmt.Println("=== Ethereum TPS Tester ===")
	fmt.Println()

	// Initialise per-level log files (logs/debug.log, info.log, warn.log, error.log)
	if err := logger.InitLogFiles(); err != nil {
		fmt.Printf("Warning: could not initialise log files: %v\n", err)
	} else {
		defer logger.CloseLogFiles()
		fmt.Println("✓ Log files initialised in logs/")
	}

	// Load .env file if it exists (optional)
	if err := godotenv.Load(); err != nil {
		logger.Debug("No .env file found, using environment variables or defaults\n")
	}

	// Load configuration
	config := LoadConfig()
	logger.SetLevel(config.LogLevel)

	// Initialize database
	logger.Info("Initializing database...\n")
	db, err := dbpkg.NewDatabase(config.DBPath, config.DBMaxOpenConns, config.DBMaxIdleConns)
	if err != nil {
		logger.Error("Error initializing database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	logger.Info("✓ Database initialized\n")

	// Cleanup old records if retention is configured
	if config.DBRetentionDays > 0 {
		logger.Info("Cleaning up records older than %d days...\n", config.DBRetentionDays)
		deleted, err := db.CleanupOldRecords(config.DBRetentionDays)
		if err != nil {
			logger.Warn("Could not cleanup old records: %v\n", err)
		} else if deleted > 0 {
			logger.Info("✓ Cleaned up %d old records\n", deleted)
		}
	}

	// Connect to RPC
	logger.Info("Connecting to RPC: %s\n", config.RPCURL)
	txSender, err := txpkg.NewTransactionSender(config.RPCURL)
	if err != nil {
		logger.Error("Error connecting to RPC: %v\n", err)
		os.Exit(1)
	}
	defer txSender.Close()
	logger.Info("✓ Connected to RPC\n")

	// Connect to WebSocket if URL is provided (for faster receipt confirmations)
	var wsManager *worker.WebSocketManager
	if config.WSURL != "" {
		logger.Info("Connecting to WebSocket: %s\n", config.WSURL)
		wsManager = worker.NewWebSocketManager(config.WSURL, config.WSReconnectDelay)
		err = wsManager.Connect()
		if err != nil {
			logger.Warn("Could not connect to WebSocket (will use RPC polling): %v\n", err)
			wsManager = nil
		} else {
			defer wsManager.Close()
			logger.Info("✓ Connected to WebSocket\n")
		}
	} else {
		logger.Debug("No WebSocket URL provided, will use RPC polling for receipts\n")
	}

	// Get or generate mnemonic
	var mnemonic string
	if config.Mnemonic != "" {
		logger.Info("\nUsing provided mnemonic...\n")
		mnemonic = config.Mnemonic
	} else {
		logger.Info("\nGenerating new mnemonic...\n")
		var err error
		mnemonic, err = wallet.GenerateMnemonic()
		if err != nil {
			logger.Error("Error generating mnemonic: %v\n", err)
			os.Exit(1)
		}
	}

	// Generate wallets from single mnemonic
	logger.Info("Deriving %d wallets from mnemonic...\n", config.WalletCount)

	wallets, err := wallet.DeriveWalletsFromMnemonic(mnemonic, config.WalletCount)
	if err != nil {
		logger.Error("Error deriving wallets: %v\n", err)
		os.Exit(1)
	}

	// Save mnemonic to file
	err = SaveMnemonicToFile("mnemonic.txt", mnemonic)
	if err != nil {
		logger.Warn("Could not save mnemonic: %v\n", err)
	}

	logger.Info("✓ Generated %d wallets\n", len(wallets))

	// Save wallets to database
	logger.Info("\nSaving wallets to database...\n")
	for _, w := range wallets {
		err := db.InsertWallet(w.Address.Hex(), w.DerivationPath)
		if err != nil {
			logger.Warn("Could not save wallet %s: %v\n", w.Address.Hex(), err)
		}
	}
	logger.Info("✓ Wallets saved to database\n")

	// Display wallet addresses and balances
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("WALLET ADDRESSES AND BALANCES")
	fmt.Println(strings.Repeat("=", 60))

	allFunded := true

	for i, w := range wallets {

		balance, err := txSender.GetBalance(context.Background(), w.Address)
		if err != nil {
			logger.Debug("[%d] %s\n", i+1, w.Address.Hex())
			logger.Error("Error fetching balance: %v\n", err)
			allFunded = false
			continue
		}

		balanceFloat := new(big.Float).SetInt(balance)
		// Convert balance to ETH for display
		logger.Info("✓ Connected to RPC\n")
		ethValue := new(big.Float).Quo(balanceFloat, big.NewFloat(1e18))

		fmt.Printf("[%d] %s\n", i+1, w.Address.Hex())
		fmt.Printf("    Balance: %s wei (%.6f ETH)\n", balance.String(), ethValue)
		logger.Info("Connecting to WebSocket: %s\n", config.WSURL)
		// Check if balance is zero
		if balance.Cmp(big.NewInt(0)) == 0 {
			logger.Warn("    ⚠️  WARNING: Wallet has ZERO balance!\n")
			logger.Warn("Could not connect to WebSocket (will use RPC polling): %v\n", err)
		}
		logger.Info("✓ Connected to WebSocket\n")
	}

	logger.Debug("No WebSocket URL provided, will use RPC polling for receipts\n")
	if !allFunded {
		fmt.Println("⚠️  WARNING: Some wallets have zero balance or errors!")
	}
	fmt.Println()

	logger.Info("\nUsing provided mnemonic...\n")
	if !config.AutomatedMode {
		fmt.Print("Do you want to proceed with sending transactions? (y/n): ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		response := strings.TrimSpace(strings.ToLower(scanner.Text()))

		if response != "y" && response != "yes" {
			fmt.Println("\nOperation cancelled by user.")
			fmt.Println("Please fund the wallets and try again.")
			os.Exit(0)
		}

	} else {
		fmt.Println("\n✓ Automated mode enabled. Proceeding with transactions...")
	}

	var receiptWG sync.WaitGroup // WaitGroup for receipt confirmations

	// Create worker pools ONCE (reused across all iterations in loop mode)
	// Calculate buffer size for channels
	bufferSize := config.BufferSize
	if bufferSize == 0 {
		// Auto-calculate from wallet and transaction counts
		bufferSize = config.WalletCount * config.TxPerWallet
		logger.Debug("Auto-calculated buffer size: %d (WalletCount %d × TxPerWallet %d)\n", bufferSize, config.WalletCount, config.TxPerWallet)
	} else {
		logger.Debug("Using configured buffer size: %d\n", bufferSize)
	}
	receiptJobChan := make(chan worker.ReceiptJob, config.WalletCount*config.TxPerWallet)
	dbWriteChan := make(chan worker.DBWriteJob, bufferSize)

	dbWriteWG := sync.WaitGroup{}

	// Start worker pools
	worker.StartReceiptWorkerPool(config.ReceiptWorkers, receiptJobChan, &receiptWG, wsManager, db, txSender)
	logger.Info("📋 Started %d receipt confirmation workers\n", config.ReceiptWorkers)
	worker.StartDBWriterPool(config.ReceiptWorkers, dbWriteChan, receiptJobChan, db, &dbWriteWG)
	logger.Info("📋 Started %d DB writer workers\n\n", config.ReceiptWorkers)

	// Check if we should run in loop mode
	if config.RunDurationMinutes > 0 {
		fmt.Printf("Running in LOOP MODE for %d minutes\n", config.RunDurationMinutes)
		fmt.Println()
		runInLoopMode(config, wallets, dbWriteChan, &dbWriteWG)
	} else {
		fmt.Println("Running in SINGLE MODE")
		fmt.Println()

		executionStart := time.Now()

		runSingleExecution(config, txSender, wallets, dbWriteChan, &dbWriteWG)

		// Calculate elapsed time and ensure minimum 1 second
		executionElapsed := time.Since(executionStart)
		minDuration := 1 * time.Second

		if executionElapsed < minDuration {
			remainingSleep := minDuration - executionElapsed
			fmt.Printf("\n⏱  Execution completed in %.6f seconds. Waiting %.6f seconds to maintain 1-second minimum...\n",
				executionElapsed.Seconds(), remainingSleep.Seconds())
		} else {
			fmt.Printf("\n⏱  Execution completed in %.6f seconds\n", executionElapsed.Seconds())
		}
	}

	// Close channels to signal workers to exit
	fmt.Println("\nClosing worker channels...")
	close(dbWriteChan)
	dbWriteWG.Wait() // Wait for DB writers to finish
	fmt.Println("✓ All database writes completed")

	close(receiptJobChan)
	fmt.Println("Waiting for receipt confirmations to finish...")
	receiptWG.Wait() // Wait for all receipt confirmations to finish
	fmt.Println("✓ All receipt confirmations completed")

	// Final summary
	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("✓ All executions completed")
	fmt.Printf("✓ Mnemonic saved to: mnemonic.txt\n")
	fmt.Printf("✓ Database: %s\n", config.DBPath)
	fmt.Println(strings.Repeat("=", 60))
}

func runInLoopMode(config *Config, wallets []*wallet.Wallet, dbWriteChan chan worker.DBWriteJob, dbWriteWG *sync.WaitGroup) {
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

		txSender, err := txpkg.NewTransactionSender(config.RPCURL)
		if err != nil {
			logger.Error("Error connecting to RPC: %v\n", err)
			os.Exit(1)
		}
		logger.Info("📋 Started %d receipt confirmation workers\n", config.ReceiptWorkers)
		runSingleExecution(config, txSender, wallets, dbWriteChan, dbWriteWG)
		logger.Info("📋 Started %d DB writer workers\n\n", config.ReceiptWorkers)
		txSender.Close()
		// Calculate elapsed time and ensure minimum 1 second per iteration
		iterationElapsed := time.Since(iterationStart)
		minDuration := 990 * time.Millisecond

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

func runSingleExecution(config *Config, txSender *txpkg.TransactionSender, wallets []*wallet.Wallet, dbWriteChan chan worker.DBWriteJob, dbWriteWG *sync.WaitGroup) {
	// Lock submission mutex to pause all workers during transaction submission
	logger.Debug("🔒 Submission phase started - workers paused\n")

	// Generate unique batch number for this execution
	batchNumber := fmt.Sprintf("batch-%s", time.Now().Format("20060102-150405"))
	fmt.Printf("Batch Number: %s\n\n", batchNumber)

	// Create context with timeout for this execution
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.ContextTimeout)*time.Second)
	defer cancel()

	// Parse configuration values
	value := new(big.Int)
	value.SetString(config.ValueWei, 10)
	toAddress := common.HexToAddress(config.ToAddress)

	logger.Info("\nTransaction Configuration:\n")
	logger.Info("  - Number of wallets: %d\n", len(wallets))
	logger.Info("  - Transactions per wallet: %d\n", config.TxPerWallet)
	logger.Info("  - Total transactions: %d\n", len(wallets)*config.TxPerWallet)
	logger.Info("  - Target address: %s\n", toAddress.Hex())
	logger.Info("  - Value per tx: %s wei\n", value.String())
	logger.Info("\n")

	// Use mutex for thread-safe counter updates
	var wgSubmit sync.WaitGroup // Wait for transaction submissions only
	// Receipt confirmations happen in background, we don't wait for them

	fmt.Println("Starting transaction submission...")
	fmt.Println(strings.Repeat("=", 60))

	// Process all wallets in parallel
	for walletIdx, w := range wallets {
		wgSubmit.Add(1)
		go func(idx int, w *wallet.Wallet) {
			defer wgSubmit.Done()

			logger.Debug("\n[Wallet %d/%d] (%s)\n",
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
				logger.Error("  Error preparing transactions: %v\n", err)
				return
			}

			// Send all transactions for this wallet
			for txIdx, req := range txRequests {
				result, err := txSender.CreateAndSendTransaction(ctx, req)

				// Create database transaction record
				dbTx := &dbpkg.Transaction{
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

					// Print failure reason
					logger.Error("  [W%d] Tx %d FAILED (nonce %d): %v\n", idx+1, txIdx+1, req.Nonce, err)

					// Queue DB write only (no receipt needed for submission failures)
					dbWriteChan <- worker.DBWriteJob{Tx: dbTx}
				} else {
					dbTx.TxHash = result.TxHash
					dbTx.Status = "pending"

					logger.Debug("  [W%d] Tx %d sent (nonce %d): %s\n", idx+1, txIdx+1, req.Nonce, result.TxHash[:16]+"...")

					// Queue DB write + receipt job together.
					// The DB writer will INSERT first, then dispatch the receipt job,
					// ensuring UPDATE never runs before INSERT.
					dbWriteChan <- worker.DBWriteJob{
						Tx: dbTx,
					}
				}
			}

			logger.Info("  [W%d] ✓ Sent %d transactions (nonce %d to %d)\n",
				idx+1,
				len(txRequests),
				txRequests[0].Nonce,
				txRequests[len(txRequests)-1].Nonce,
			)
		}(walletIdx, w)
	}

	// Wait for transaction submissions to complete
	fmt.Println("\nWaiting for all transactions to be submitted...")
	wgSubmit.Wait()
	fmt.Println("✓ All transactions submitted")

	logger.Debug("🔓 Submission phase completed - workers resumed\n")

	fmt.Println("✓ Database writes queued (processing in background)")
	fmt.Println("✓ Receipt confirmations queued (processing in background)")

	// Return immediately after transactions are submitted; analysis and summaries
	// can be performed later using the provided tooling (e.g. analyze.sh).
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
		AutomatedMode:      getEnvBool("AUTOMATED_MODE", DefaultAutomatedMode),
		ContextTimeout:     getEnvInt("CONTEXT_TIMEOUT", DefaultContextTimeout),
		DBRetentionDays:    getEnvInt("DB_RETENTION_DAYS", DefaultDBRetentionDays),
		WSReconnectDelay:   getEnvInt("WS_RECONNECT_DELAY", DefaultWSReconnectDelay),
		BufferSize:         getEnvInt("BUFFER_SIZE", DefaultBufferSize),
		DBMaxOpenConns:     getEnvInt("DB_MAX_OPEN_CONNS", DefaultDBMaxOpenConns),
		DBMaxIdleConns:     getEnvInt("DB_MAX_IDLE_CONNS", DefaultDBMaxIdleConns),
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

func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	switch strings.ToLower(value) {
	case "true", "1", "yes", "y":
		return true
	case "false", "0", "no", "n":
		return false
	default:
		fmt.Printf("Warning: Invalid boolean value for %s: '%s', using default: %v\n", key, value, defaultValue)
		return defaultValue
	}
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
