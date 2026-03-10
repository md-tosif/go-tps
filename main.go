package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/joho/godotenv"
	"gopkg.in/natefinch/lumberjack.v2"
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
	DefaultReceiptWorkers     = 10      // Number of concurrent workers for receipt confirmation
	DefaultLogLevel           = "DEBUG" // DEBUG, INFO, WARN, ERROR
	DefaultAutomatedMode      = false   // true = skip user confirmation
	DefaultContextTimeout     = 30      // seconds for RPC calls
	DefaultConnectionRefresh  = 100     // refresh connection every N jobs
	DefaultDBRetentionDays    = 30      // cleanup records older than this
	DefaultWSReconnectDelay   = 5       // seconds before reconnecting WebSocket
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

// Per-level file loggers (nil until initLogFiles is called)
var (
	fileLoggers [4]*log.Logger // indexed by LogLevel: DEBUG=0, INFO=1, WARN=2, ERROR=3
	logFiles    [4]*os.File
)

// initLogFiles creates the logs directory and opens one log file per level with rotation.
// Each file is appended to across runs and contains a timestamp prefix per line.
// Uses lumberjack for automatic log rotation (100MB max size, 3 backups, 28 days retention).
func initLogFiles() error {
	if err := os.MkdirAll("logs", 0o755); err != nil {
		return fmt.Errorf("could not create logs directory: %w", err)
	}

	names := [4]string{"logs/debug.log", "logs/info.log", "logs/warn.log", "logs/error.log"}
	for i, name := range names {
		// Use lumberjack for automatic log rotation
		logger := &lumberjack.Logger{
			Filename:   name,
			MaxSize:    100, // megabytes
			MaxBackups: 3,
			MaxAge:     28,   // days
			Compress:   true, // compress old logs
		}
		fileLoggers[i] = log.New(logger, "", log.Ldate|log.Ltime|log.Lmicroseconds)
	}
	return nil
}

// closeLogFiles flushes and closes all open log files.
func closeLogFiles() {
	// Lumberjack loggers are closed automatically via garbage collection
	// No explicit close needed
}

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
	if fileLoggers[DEBUG] != nil {
		fileLoggers[DEBUG].Printf("[DEBUG] "+format, args...)
	}
	if currentLogLevel <= DEBUG {
		fmt.Printf("[DEBUG] "+format, args...)
	}
}

func logInfo(format string, args ...interface{}) {
	if fileLoggers[INFO] != nil {
		fileLoggers[INFO].Printf("[INFO] "+format, args...)
	}
	if currentLogLevel <= INFO {
		fmt.Printf("[INFO] "+format, args...)
	}
}

func logWarn(format string, args ...interface{}) {
	if fileLoggers[WARN] != nil {
		fileLoggers[WARN].Printf("[WARN] "+format, args...)
	}
	if currentLogLevel <= WARN {
		fmt.Printf("[WARN] "+format, args...)
	}
}

func logError(format string, args ...interface{}) {
	if fileLoggers[ERROR] != nil {
		fileLoggers[ERROR].Printf("[ERROR] "+format, args...)
	}
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
	AutomatedMode      bool // Skip user confirmation if true
	ContextTimeout     int  // Timeout for RPC calls in seconds
	ConnectionRefresh  int  // Refresh connections every N jobs
	DBRetentionDays    int  // Cleanup records older than this
	WSReconnectDelay   int  // Seconds before reconnecting WebSocket
}

// ReceiptJob represents a receipt confirmation job
type ReceiptJob struct {
	DB         *Database
	RPCURL     string
	WSClient   *ethclient.Client
	TxHash     string
	Nonce      uint64
	StartTime  time.Time
	WalletNum  int
	RetryCount int // number of times this job has been retried due to timeout
}

// WebSocketManager handles WebSocket connection with automatic reconnection
type WebSocketManager struct {
	client         *ethclient.Client
	url            string
	reconnectMu    sync.Mutex
	reconnecting   bool
	reconnectDelay time.Duration
}

// NewWebSocketManager creates a new WebSocket manager
func NewWebSocketManager(url string, reconnectDelay int) *WebSocketManager {
	return &WebSocketManager{
		url:            url,
		reconnectDelay: time.Duration(reconnectDelay) * time.Second,
	}
}

// Connect establishes a WebSocket connection
func (wm *WebSocketManager) Connect() error {
	client, err := ethclient.Dial(wm.url)
	if err != nil {
		return err
	}
	wm.client = client
	return nil
}

// GetClient returns the current client (may trigger reconnection)
func (wm *WebSocketManager) GetClient() *ethclient.Client {
	wm.reconnectMu.Lock()
	defer wm.reconnectMu.Unlock()
	return wm.client
}

// Reconnect attempts to reconnect the WebSocket
func (wm *WebSocketManager) Reconnect() error {
	wm.reconnectMu.Lock()
	defer wm.reconnectMu.Unlock()

	if wm.reconnecting {
		return fmt.Errorf("reconnection already in progress")
	}

	wm.reconnecting = true
	defer func() { wm.reconnecting = false }()

	logWarn("WebSocket disconnected, attempting reconnection in %v...\n", wm.reconnectDelay)
	time.Sleep(wm.reconnectDelay)

	if wm.client != nil {
		wm.client.Close()
	}

	client, err := ethclient.Dial(wm.url)
	if err != nil {
		return err
	}

	wm.client = client
	logInfo("✓ WebSocket reconnected successfully\n")
	return nil
}

// Close closes the WebSocket connection
func (wm *WebSocketManager) Close() {
	wm.reconnectMu.Lock()
	defer wm.reconnectMu.Unlock()
	if wm.client != nil {
		wm.client.Close()
	}
}

func main() {
	fmt.Println("=== Ethereum TPS Tester ===")
	fmt.Println()

	// Initialise per-level log files (logs/debug.log, info.log, warn.log, error.log)
	if err := initLogFiles(); err != nil {
		fmt.Printf("Warning: could not initialise log files: %v\n", err)
	} else {
		defer closeLogFiles()
		fmt.Println("✓ Log files initialised in logs/")
	}

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

	// Cleanup old records if retention is configured
	if config.DBRetentionDays > 0 {
		logInfo("Cleaning up records older than %d days...\n", config.DBRetentionDays)
		deleted, err := db.CleanupOldRecords(config.DBRetentionDays)
		if err != nil {
			logWarn("Could not cleanup old records: %v\n", err)
		} else if deleted > 0 {
			logInfo("✓ Cleaned up %d old records\n", deleted)
		}
	}

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
	var wsManager *WebSocketManager
	if config.WSURL != "" {
		logInfo("Connecting to WebSocket: %s\n", config.WSURL)
		wsManager = NewWebSocketManager(config.WSURL, config.WSReconnectDelay)
		err = wsManager.Connect()
		if err != nil {
			logWarn("Could not connect to WebSocket (will use RPC polling): %v\n", err)
			wsManager = nil
		} else {
			defer wsManager.Close()
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

	wallets, err := DeriveWalletsFromMnemonic(mnemonic, config.WalletCount, txSender)
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

	// Ask for user confirmation (only once) unless in automated mode
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

		fmt.Println("\n✓ User confirmed. Proceeding with transactions...")
	} else {
		fmt.Println("\n✓ Automated mode enabled. Proceeding with transactions...")
	}
	fmt.Println()

	var receiptWG sync.WaitGroup // WaitGroup for receipt confirmations

	// Create worker pools ONCE (reused across all iterations in loop mode)
	// Calculate buffer size for channels
	bufferSize := config.WalletCount * config.TxPerWallet
	receiptJobChan := make(chan ReceiptJob, bufferSize)
	dbWriteChan := make(chan DBWriteJob, bufferSize)
	var dbWriteWG sync.WaitGroup

	// Start worker pools
	startReceiptWorkerPool(config.ReceiptWorkers, receiptJobChan, &receiptWG)
	logInfo("📋 Started %d receipt confirmation workers\n", config.ReceiptWorkers)
	startDBWriterPool(config.ReceiptWorkers, dbWriteChan, receiptJobChan, db, &dbWriteWG)
	logInfo("📋 Started %d DB writer workers\n\n", config.ReceiptWorkers)

	// Check if we should run in loop mode
	if config.RunDurationMinutes > 0 {
		fmt.Printf("Running in LOOP MODE for %d minutes\n", config.RunDurationMinutes)
		fmt.Println()
		runInLoopMode(config, db, txSender, wsManager, wallets, dbWriteChan, &dbWriteWG)
	} else {
		fmt.Println("Running in SINGLE MODE")
		fmt.Println()

		// Record start time for single execution
		executionStart := time.Now()

		runSingleExecution(config, db, txSender, wsManager, wallets, dbWriteChan, &dbWriteWG)

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

func runInLoopMode(config *Config, db *Database, txSender *TransactionSender, wsManager *WebSocketManager, wallets []*Wallet, dbWriteChan chan DBWriteJob, dbWriteWG *sync.WaitGroup) {
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

		runSingleExecution(config, db, txSender, wsManager, wallets, dbWriteChan, dbWriteWG)

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

func runSingleExecution(config *Config, db *Database, txSender *TransactionSender, wsManager *WebSocketManager, wallets []*Wallet, dbWriteChan chan DBWriteJob, dbWriteWG *sync.WaitGroup) {
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

					// Queue DB write only (no receipt needed for submission failures)
					dbWriteChan <- DBWriteJob{Tx: dbTx}
				} else {
					dbTx.TxHash = result.TxHash
					dbTx.Status = "pending"

					logDebug("  [W%d] Tx %d sent (nonce %d): %s\n", idx+1, txIdx+1, req.Nonce, result.TxHash[:16]+"...")

					mu.Lock()
					totalTransactions++
					totalSuccessful++
					mu.Unlock()

					// Queue DB write + receipt job together.
					// The DB writer will INSERT first, then dispatch the receipt job,
					// ensuring UPDATE never runs before INSERT.
					var wsClient *ethclient.Client
					if wsManager != nil {
						wsClient = wsManager.GetClient()
					}
					dbWriteChan <- DBWriteJob{
						Tx: dbTx,
						ReceiptJob: &ReceiptJob{
							DB:        db,
							RPCURL:    config.RPCURL,
							WSClient:  wsClient,
							TxHash:    result.TxHash,
							Nonce:     req.Nonce,
							StartTime: result.SubmittedAt,
							WalletNum: idx + 1,
						},
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

	// Wait for transaction submissions to complete
	fmt.Println("\nWaiting for all transactions to be submitted...")
	// wgSubmit.Wait()
	fmt.Println("✓ All transactions submitted")
	fmt.Println("✓ Database writes queued (processing in background)")
	fmt.Println("✓ Receipt confirmations queued (processing in background)")

	totalTime := time.Since(startTime)

	// Launch background goroutine to print summary (non-blocking)
	// This allows the next iteration to start immediately in loop mode
	go func() {
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
		fmt.Printf("✓ All data queued for database: %s\n", config.DBPath)
		fmt.Println()
		fmt.Println("Note: DB writes and receipt confirmations continue in background")
	}()

	// Return immediately after transactions are submitted (don't wait for summary)
}

// DBWriteJob bundles a transaction insert with an optional follow-up receipt job.
// The receipt job (when non-nil) is dispatched to receiptJobChan only AFTER
// the INSERT succeeds, preventing the UPDATE-before-INSERT race condition.
type DBWriteJob struct {
	Tx         *Transaction
	ReceiptJob *ReceiptJob // nil for failed/no-receipt transactions
}

// startDBWriterPool starts a pool of workers that serialize inserts into SQLite.
// Use workerCount=1 to avoid "database is locked" errors with SQLite.
func startDBWriterPool(workerCount int, jobChan <-chan DBWriteJob, receiptJobChan chan ReceiptJob, db *Database, wg *sync.WaitGroup) {
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go dbWriterWorker(i+1, jobChan, receiptJobChan, db, wg)
	}
}

// dbWriterWorker drains the DB write channel, inserting each transaction record,
// then forwarding any associated receipt job to the receipt worker pool.
func dbWriterWorker(workerID int, jobChan <-chan DBWriteJob, receiptJobChan chan ReceiptJob, db *Database, wg *sync.WaitGroup) {
	defer wg.Done()
	for job := range jobChan {
		if _, err := db.InsertTransaction(job.Tx); err != nil {
			logWarn("[DBWriter %d] Could not save transaction to DB: %v\n", workerID, err)
			continue
		}
		// Only dispatch the receipt job AFTER the INSERT is confirmed
		if job.ReceiptJob != nil {
			receiptJobChan <- *job.ReceiptJob
		}
	}
}

// startReceiptWorkerPool starts a pool of workers to process receipt confirmations
func startReceiptWorkerPool(workerCount int, jobChan chan ReceiptJob, wg *sync.WaitGroup) {
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go receiptWorker(i+1, jobChan, wg)
	}
}

const maxReceiptRetries = 3

// receiptWorker processes receipt confirmation jobs from the job channel
// with periodic connection refresh to prevent stale connections
func receiptWorker(workerID int, jobChan chan ReceiptJob, wg *sync.WaitGroup) {
	defer wg.Done()

	var txSender *TransactionSender
	var currentRPCURL string
	jobsProcessed := 0
	connectionRefreshInterval := 100 // Refresh connection every 100 jobs

	// Process jobs from channel
	for job := range jobChan {
		// Initialize, refresh, or reuse RPC connection
		needsRefresh := txSender == nil ||
			currentRPCURL != job.RPCURL ||
			jobsProcessed >= connectionRefreshInterval

		if needsRefresh {
			if txSender != nil {
				logDebug("[Worker %d] Refreshing RPC connection after %d jobs\n", workerID, jobsProcessed)
				txSender.Close()
			}
			var err error
			txSender, err = NewTransactionSender(job.RPCURL)
			if err != nil {
				logError("[Worker %d] Could not connect to RPC: %v\n", workerID, err)
				continue
			}
			currentRPCURL = job.RPCURL
			jobsProcessed = 0
		}

		shouldRetry := processReceiptJob(workerID, txSender, job)
		jobsProcessed++

		if shouldRetry {
			if job.RetryCount < maxReceiptRetries {
				job.RetryCount++
				logWarn("  [Worker %d] Re-queuing tx (nonce %d) for retry %d/%d\n", workerID, job.Nonce, job.RetryCount, maxReceiptRetries)
				jobChan <- job
			} else {
				logError("  [Worker %d] Tx (nonce %d) exceeded max retries (%d), marking failed\n", workerID, job.Nonce, maxReceiptRetries)
				job.DB.UpdateTransactionStatus(job.TxHash, "failed", nil, 0, "", "timeout after max retries")
			}
		}
	}

	// Cleanup RPC connection
	if txSender != nil {
		logDebug("[Worker %d] Closing RPC connection (%d jobs processed)\n", workerID, jobsProcessed)
		txSender.Close()
	}
}

// processReceiptJob processes a single receipt confirmation job.
// Returns true if the job should be retried (i.e. it timed out).
func processReceiptJob(workerID int, txSender *TransactionSender, job ReceiptJob) bool {
	// Wait for receipt with timeout - use shared WebSocket if available
	ctx := context.Background()
	receipt, receiptErr := txSender.WaitForReceiptWithSharedWebSocket(ctx, job.WSClient, common.HexToHash(job.TxHash), 60*time.Second)

	if receiptErr != nil {
		// If this was a timeout, signal to the caller to re-queue the job
		if strings.Contains(receiptErr.Error(), "timeout waiting for transaction receipt") {
			logWarn("  [W%d] Tx (nonce %d): ⏱ timed out (retry %d/%d)\n", job.WalletNum, job.Nonce, job.RetryCount+1, maxReceiptRetries)
			return true
		}
		// For non-timeout errors, mark as failed immediately
		job.DB.UpdateTransactionStatus(job.TxHash, "failed", nil, 0, "", receiptErr.Error())
		logWarn("  [W%d] Tx (nonce %d): ✗ error - %v\n", job.WalletNum, job.Nonce, receiptErr)
		return false
	} else {
		// Get block header to retrieve block timestamp
		blockHeader, err := txSender.client.HeaderByHash(ctx, receipt.BlockHash)
		var confirmedAt time.Time
		if err != nil {
			// Fallback to current time if block fetch fails
			logWarn("  [W%d] Could not fetch block header, using current time: %v\n", job.WalletNum, err)
			confirmedAt = time.Now()
		} else {
			// Use block creation time from the receipt's block
			confirmedAt = time.Unix(int64(blockHeader.Time), 0)
		}

		// Check for negative/zero block timestamp relative to submission time
		if confirmedAt.Before(job.StartTime) {
			logWarn("  [W%d] Block timestamp before submission time, adjusting\n", job.WalletNum)
			confirmedAt = job.StartTime.Add(1 * time.Second)
		}

		// Extract gas information from receipt
		gasUsed := receipt.GasUsed
		effectiveGasPrice := ""
		if receipt.EffectiveGasPrice != nil {
			effectiveGasPrice = receipt.EffectiveGasPrice.String()
		}

		confirmationTime := confirmedAt.Sub(job.StartTime).Seconds()
		if receipt.Status == 1 {
			job.DB.UpdateTransactionStatus(job.TxHash, "success", &confirmedAt, gasUsed, effectiveGasPrice, "")
			logInfo("  [W%d] Tx (nonce %d): ✓ confirmed in %.2fs (gas: %d)\n", job.WalletNum, job.Nonce, confirmationTime, gasUsed)
		} else {
			job.DB.UpdateTransactionStatus(job.TxHash, "failed", &confirmedAt, gasUsed, effectiveGasPrice, "transaction reverted")
			logWarn("  [W%d] Tx (nonce %d): ✗ reverted (transaction failed on-chain)\n", job.WalletNum, job.Nonce)
		}
		return false
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
		AutomatedMode:      getEnvBool("AUTOMATED_MODE", DefaultAutomatedMode),
		ContextTimeout:     getEnvInt("CONTEXT_TIMEOUT", DefaultContextTimeout),
		ConnectionRefresh:  getEnvInt("CONNECTION_REFRESH", DefaultConnectionRefresh),
		DBRetentionDays:    getEnvInt("DB_RETENTION_DAYS", DefaultDBRetentionDays),
		WSReconnectDelay:   getEnvInt("WS_RECONNECT_DELAY", DefaultWSReconnectDelay),
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
