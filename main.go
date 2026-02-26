package main

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/joho/godotenv"
)

const (
	DefaultRPCURL      = "http://localhost:8545"
	DefaultDBPath      = "./transactions.db"
	DefaultWalletCount = 10

	DefaultTxPerWallet = 10
	DefaultValueWei    = "1000000000000000" // 0.001 ETH
	DefaultToAddress   = "0x0000000000000000000000000000000000000001"
)

type Config struct {
	RPCURL      string
	DBPath      string
	Mnemonic    string
	WalletCount int
	TxPerWallet int
	ValueWei    string
	ToAddress   string
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
	ctx := context.Background()
	totalTransactions := 0
	totalSuccessful := 0
	totalFailed := 0
	startTime := time.Now()

	// Use mutex for thread-safe counter updates
	var mu sync.Mutex
	var wg sync.WaitGroup

	fmt.Println("Starting transaction submission...")
	fmt.Println(strings.Repeat("=", 60))

	// Process all wallets in parallel
	for walletIdx, wallet := range wallets {
		wg.Add(1)
		go func(idx int, w *Wallet) {
			defer wg.Done()

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

					// Save to database
					db.InsertTransaction(dbTx)
				} else {
					dbTx.TxHash = result.TxHash
					dbTx.Status = "pending"

					// Save initial pending status to database
					txID, dbErr := db.InsertTransaction(dbTx)
					if dbErr != nil {
						fmt.Printf("  Warning: Could not save transaction to DB: %v\n", dbErr)
					}

					fmt.Printf("  [W%d] Tx %d sent (nonce %d): %s\n", idx+1, txIdx+1, req.Nonce, result.TxHash[:16]+"...")

					mu.Lock()
					totalTransactions++
					mu.Unlock()

					// Launch goroutine to wait for receipt (non-blocking)
					wg.Add(1)
					go func(txHash string, txID int64, nonce uint64, startTime time.Time, walletNum int) {
						defer wg.Done()

						receipt, receiptErr := txSender.WaitForReceipt(ctx, common.HexToHash(txHash), 120*time.Second)

						// Update database with final status
						confirmedAt := time.Now()
						execTime := confirmedAt.Sub(startTime).Seconds() * 1000

						if receiptErr != nil {
							db.UpdateTransactionStatus(txHash, "failed", nil, execTime, receiptErr.Error())
							mu.Lock()
							totalFailed++
							mu.Unlock()
							fmt.Printf("  [W%d] Tx (nonce %d): ✗ timeout/error\n", walletNum, nonce)
						} else {
							if receipt.Status == 1 {
								db.UpdateTransactionStatus(txHash, "success", &confirmedAt, execTime, "")
								mu.Lock()
								totalSuccessful++
								mu.Unlock()
								fmt.Printf("  [W%d] Tx (nonce %d): ✓ confirmed in %.2fs\n", walletNum, nonce, execTime/1000)
							} else {
								db.UpdateTransactionStatus(txHash, "failed", &confirmedAt, execTime, "transaction reverted")
								mu.Lock()
								totalFailed++
								mu.Unlock()
								fmt.Printf("  [W%d] Tx (nonce %d): ✗ reverted\n", walletNum, nonce)
							}
						}
					}(result.TxHash, txID, req.Nonce, result.SubmittedAt, idx+1)
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

	// Wait for all receipts
	fmt.Println("\nWaiting for all transactions to be confirmed...")
	wg.Wait()
	fmt.Println("✓ All transactions processed")

	totalTime := time.Since(startTime)

	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("=== Execution Summary ===")
	fmt.Println()
	fmt.Printf("Total transactions submitted: %d\n", totalTransactions)
	fmt.Printf("Successful: %d\n", totalSuccessful)
	fmt.Printf("Failed: %d\n", totalFailed)
	fmt.Printf("Total execution time: %.2f seconds\n", totalTime.Seconds())
	fmt.Printf("Average time per transaction: %.2f ms\n",
		totalTime.Seconds()*1000/float64(totalTransactions))
	fmt.Printf("Transactions per second: %.2f\n",
		float64(totalTransactions)/totalTime.Seconds())
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
	fmt.Printf("✓ Mnemonic saved to: mnemonic.txt\n")
	fmt.Println()
	fmt.Println("Done!")
}

func LoadConfig() *Config {
	// Load from environment variables or use defaults
	config := &Config{
		RPCURL:      getEnv("RPC_URL", DefaultRPCURL),
		DBPath:      getEnv("DB_PATH", DefaultDBPath),
		Mnemonic:    getEnv("MNEMONIC", ""),
		WalletCount: getEnvInt("WALLET_COUNT", DefaultWalletCount),
		TxPerWallet: getEnvInt("TX_PER_WALLET", DefaultTxPerWallet),
		ValueWei:    getEnv("VALUE_WEI", DefaultValueWei),
		ToAddress:   getEnv("TO_ADDRESS", DefaultToAddress),
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
