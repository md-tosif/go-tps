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

	"go-tps/config"
	dbpkg "go-tps/db"
	"go-tps/logger"
	txpkg "go-tps/tx"
	"go-tps/wallet"
	"go-tps/worker"

	"github.com/ethereum/go-ethereum/common"
	"github.com/joho/godotenv"
)

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
	config := config.LoadConfig()
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

	// Create context with timeout for database and RPC operations
	setupCtx, setupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer setupCancel()

	// Save wallets to database
	logger.Info("\nSaving wallets to database...\n")
	for _, w := range wallets {
		err := db.InsertWallet(setupCtx, w.Address.Hex(), w.DerivationPath)
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
		balance, err := txSender.GetBalance(setupCtx, w.Address)
		if err != nil {
			logger.Debug("[%d] %s\n", i+1, w.Address.Hex())
			logger.Error("Error fetching balance: %v\n", err)
			allFunded = false
			continue
		}

		balanceFloat := new(big.Float).SetInt(balance)
		// Convert balance to ETH for display
		ethValue := new(big.Float).Quo(balanceFloat, big.NewFloat(1e18))

		fmt.Printf("[%d] %s\n", i+1, w.Address.Hex())
		fmt.Printf("    Balance: %s wei (%.6f ETH)\n", balance.String(), ethValue)
		// Check if balance is zero
		if balance.Cmp(big.NewInt(0)) == 0 {
			logger.Warn("    ⚠️  WARNING: Wallet has ZERO balance!\n")
		}
	}

	if !allFunded {
		fmt.Println("⚠️  WARNING: Some wallets have zero balance or errors!")
	}
	fmt.Println()

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
	// Calculate DB buffer size
	dbBufferSize := config.DBBufferSize
	if dbBufferSize == 0 {
		// Auto-calculate from wallet and transaction counts
		dbBufferSize = config.WalletCount * config.TxPerWallet
		logger.Debug("Auto-calculated DB buffer size: %d (WalletCount %d × TxPerWallet %d)\n", dbBufferSize, config.WalletCount, config.TxPerWallet)
	} else {
		logger.Debug("Using configured DB buffer size: %d\n", dbBufferSize)
	}
	// Always ensure the buffer is at least large enough for all transactions so
	// wallet goroutines never block on a full channel while DB writers are slow.
	if minBuf := config.WalletCount * config.TxPerWallet; dbBufferSize < minBuf {
		logger.Debug("Expanding DB buffer size from %d to %d (WalletCount × TxPerWallet)\n", dbBufferSize, minBuf)
		dbBufferSize = minBuf
	}
	dbWriteChan := make(chan worker.DBWriteJob, dbBufferSize)

	// Calculate Receipt buffer size
	receiptBufferSize := config.ReceiptBufferSize
	if receiptBufferSize == 0 {
		// Auto-calculate - typically needs to handle pending transactions from DB
		receiptBufferSize = 10000 // Default for batched processing
		logger.Debug("Auto-calculated receipt buffer size: %d\n", receiptBufferSize)
	} else {
		logger.Debug("Using configured receipt buffer size: %d\n", receiptBufferSize)
	}

	dbWriteWG := sync.WaitGroup{}

	worker.StartDBWriterPool(config.DBWorkers, dbWriteChan, db, &dbWriteWG)
	logger.Info("📋 Started %d DB writer workers\n\n", config.DBWorkers)

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

	receiptJobChan := make(chan worker.ReceiptJob, receiptBufferSize)

	// Start worker pools
	worker.StartReceiptWorkerPool(config.ReceiptWorkers, receiptJobChan, &receiptWG, wsManager, db, txSender)
	logger.Info("📋 Started %d receipt confirmation workers\n", config.ReceiptWorkers)

	// Queue pending transactions for receipt processing
	if err := worker.QueuePendingTransactionsForReceipt(db, receiptJobChan); err != nil {
		logger.Error("Error queuing pending transactions: %v\n", err)
	}

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

func runInLoopMode(config *config.Config, wallets []*wallet.Wallet, dbWriteChan chan worker.DBWriteJob, dbWriteWG *sync.WaitGroup) {
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
		runSingleExecution(config, txSender, wallets, dbWriteChan, dbWriteWG)
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

func runSingleExecution(config *config.Config, txSender *txpkg.TransactionSender, wallets []*wallet.Wallet, dbWriteChan chan worker.DBWriteJob, dbWriteWG *sync.WaitGroup) {
	// Lock submission mutex to pause all workers during transaction submission
	logger.Debug("🔒 Submission phase started - workers paused\n")

	// Generate unique batch number for this execution
	batchNumber := fmt.Sprintf("batch-%s", time.Now().Format("20060102-150405"))
	fmt.Printf("Batch Number: %s\n\n", batchNumber)

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

	// Gas price adjustment mechanism for underpriced errors
	var gasPriceMultiplier float64 = 1.0
	var gasPriceMutex sync.RWMutex

	// Function to get current adjusted gas price
	getAdjustedGasPrice := func(baseGasPrice *big.Int) *big.Int {
		gasPriceMutex.RLock()
		multiplier := gasPriceMultiplier
		gasPriceMutex.RUnlock()

		if multiplier == 1.0 {
			return baseGasPrice
		}

		// Apply multiplier: basePrice * multiplier
		multiplierBig := big.NewFloat(multiplier)
		basePriceFloat := new(big.Float).SetInt(baseGasPrice)
		adjustedFloat := new(big.Float).Mul(basePriceFloat, multiplierBig)

		adjustedPrice, _ := adjustedFloat.Int(nil)

		// if adjusted price is lower than 2 gwei then assign 2 gwei to avoid underpriced errors
		minGasPrice := big.NewInt(2000000000) // 2 gwei
		if adjustedPrice.Cmp(minGasPrice) < 0 {
			return minGasPrice
		}

		return adjustedPrice
	}

	// Function to increase gas price due to underpriced errors
	increaseGasPrice := func() {
		gasPriceMutex.Lock()
		defer gasPriceMutex.Unlock()

		oldMultiplier := gasPriceMultiplier
		gasPriceMultiplier *= 1.10 // Increase by 10%

		logger.Warn("Gas price multiplier increased from %.2f to %.2f due to underpriced errors\n",
			oldMultiplier, gasPriceMultiplier)
	}

	// Use mutex for thread-safe counter updates
	var wgSubmit sync.WaitGroup // Wait for transaction submissions only
	// Receipt confirmations happen in background, we don't wait for them

	fmt.Println("Starting transaction submission...")
	fmt.Println(strings.Repeat("=", 60))

	ctx, wCancel := context.WithTimeout(context.Background(), time.Duration(config.ContextTimeout)*time.Second)
	defer wCancel()

	feeHistory, feeErr := txSender.FeeHistory(ctx)

	var currentBaseFee *big.Int
	if feeErr != nil {
		logger.Warn("Error fetching fee history: %v\n", feeErr)
	} else {
		currentBaseFee = feeHistory.BaseFee[len(feeHistory.BaseFee)-1]
		logger.Debug("Current base fee from fee history: %s wei\n", currentBaseFee.String())
	}

	// Process all wallets in parallel
	for walletIdx, w := range wallets {
		wgSubmit.Add(1)
		go func(idx int, w *wallet.Wallet) {
			defer wgSubmit.Done()

			logger.Info("[Wallet %d/%d] Starting goroutine for %s\n",
				idx+1, len(wallets), w.Address.Hex())

			logger.Debug("\n[Wallet %d/%d] (%s)\n",
				idx+1, len(wallets), w.Address.Hex())

			// Each wallet gets its own context so a slow wallet cannot
			// consume the shared timeout and stall all other goroutines.
			wCtx, wCancel := context.WithTimeout(context.Background(), time.Duration(config.ContextTimeout)*time.Second)
			defer wCancel()

			// Prepare batch transactions with precalculated nonces
			logger.Debug("[Wallet %d/%d] Preparing batch transactions...\n", idx+1, len(wallets))

			// Get fresh nonce for this wallet (critical for avoiding "nonce too low" errors)
			freshNonce, nonceErr := txSender.GetNonce(wCtx, w.Address)
			if nonceErr != nil {
				logger.Error("[Wallet %d/%d] Error fetching fresh nonce: %v\n", idx+1, len(wallets), nonceErr)
				return
			}
			logger.Debug("[Wallet %d/%d] Fresh nonce from network: %d\n", idx+1, len(wallets), freshNonce)

			// Update wallet with fresh nonce before preparing transactions
			w.Nonce = freshNonce

			// Use adjusted gas price based on current multiplier
			var baseGasPrice *big.Int
			if currentBaseFee != nil {
				baseGasPrice = currentBaseFee
			} else {
				// Fallback gas price if fee history is unavailable (20 gwei)
				baseGasPrice = big.NewInt(20000000000)
				logger.Debug("[Wallet %d/%d] Using fallback gas price: %s wei\n", idx+1, len(wallets), baseGasPrice.String())
			}
			adjustedGasPrice := getAdjustedGasPrice(baseGasPrice)

			if adjustedGasPrice.Cmp(baseGasPrice) != 0 {
				logger.Debug("[Wallet %d/%d] Using adjusted gas price: %s wei (base: %s wei)\n",
					idx+1, len(wallets), adjustedGasPrice.String(), baseGasPrice.String())
			}

			txRequests, err := txSender.PrepareBatchTransactions(
				wCtx,
				w,
				toAddress,
				value,
				config.TxPerWallet,
				adjustedGasPrice,
			)

			if err != nil {
				logger.Error("[Wallet %d/%d] Error preparing transactions: %v\n", idx+1, len(wallets), err)
				return
			}
			logger.Debug("[Wallet %d/%d] Successfully prepared %d transactions\n", idx+1, len(wallets), len(txRequests))

			// Sleep until next minute boundary if configured
			if config.SleepMinutes > 0 {
				now := time.Now()
				// Calculate next minute boundary
				nextMinute := now.Truncate(time.Minute).Add(time.Minute)
				waitDuration := time.Until(nextMinute)

				fmt.Printf("Current time: %s\n", now.Format("15:04:05"))
				fmt.Printf("[Wallet %d/%d] Waiting %.1f seconds until next minute (%s)...\n",
					idx+1, len(wallets), waitDuration.Seconds(), nextMinute.Format("15:04:05"))
				time.Sleep(waitDuration)
				fmt.Println("Sleep completed. Starting transaction submission...")
			}

			// Send all transactions for this wallet
			for txIdx, req := range txRequests {
				// Per-transaction context so one hung RPC call doesn't block
				// the wallet goroutine longer than ContextTimeout seconds.
				txCtx, txCancel := context.WithTimeout(context.Background(), time.Duration(config.ContextTimeout)*time.Second)
				result, err := txSender.CreateAndSendTransaction(txCtx, req)
				txCancel()

				// Guard against nil result (returned when CreateTransaction or
				// SignTransaction fails before any RPC call is made).
				var submittedAt time.Time
				var execTime float64
				if result != nil {
					submittedAt = result.SubmittedAt
					execTime = result.ExecutionTime
				} else {
					submittedAt = time.Now()
				}

				// Create database transaction record
				dbTx := &dbpkg.Transaction{
					BatchNumber:   batchNumber,
					WalletAddress: w.Address.Hex(),
					Nonce:         req.Nonce,
					ToAddress:     toAddress.Hex(),
					Value:         value.String(),
					GasLimit:      req.GasLimit,
					SubmittedAt:   submittedAt,
					ExecutionTime: execTime,
				}

				if err != nil {
					dbTx.Status = "failed"
					dbTx.Error = err.Error()

					// Check for specific error types that indicate gas price issues
					isUnderpriced := strings.Contains(err.Error(), "replacement transaction underpriced") ||
						strings.Contains(err.Error(), "transaction underpriced") ||
						strings.Contains(err.Error(), "insufficient funds for gas")

					if isUnderpriced {
						logger.Warn("  [W%d] Gas price issue for wallet %s (error: %s)\n", idx+1, w.Address.Hex(), err.Error())
						increaseGasPrice()
					}

					// For nonce errors, log the expected vs actual nonce for debugging
					if strings.Contains(err.Error(), "nonce too low") {
						logger.Warn("  [W%d] Nonce conflict for wallet %s: %s (tx nonce: %d)\n",
							idx+1, w.Address.Hex(), err.Error(), req.Nonce)
					}

					// Print failure reason
					logger.Error("  [W%d] Tx %d FAILED (nonce %d): %v\n", idx+1, txIdx+1, req.Nonce, err)
				} else {
					dbTx.TxHash = result.TxHash
					dbTx.Status = "pending"

					logger.Debug("  [W%d] Tx %d sent (nonce %d): %s\n", idx+1, txIdx+1, req.Nonce, result.TxHash[:16]+"...")
				}

				// Queue DB write. Use a select so the goroutine can exit
				// if the process is shutting down instead of blocking forever.
				select {
				case dbWriteChan <- worker.DBWriteJob{Tx: dbTx}:
				case <-wCtx.Done():
					logger.Warn("  [W%d] Context expired while queuing DB write for nonce %d; dropping record\n", idx+1, req.Nonce)
					return
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
