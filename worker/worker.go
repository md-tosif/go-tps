package worker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go-tps/db"
	"go-tps/logger"
	"go-tps/tx"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type ReceiptJob struct {
	TxHash     string
	Nonce      uint64
	StartTime  time.Time
	RetryCount int
}

type WebSocketManager struct {
	client         *ethclient.Client
	url            string
	reconnectMu    sync.Mutex
	reconnecting   bool
	reconnectDelay time.Duration
}

func NewWebSocketManager(url string, reconnectDelay int) *WebSocketManager {
	return &WebSocketManager{
		url:            url,
		reconnectDelay: time.Duration(reconnectDelay) * time.Second,
	}
}

func (wm *WebSocketManager) Connect() error {
	client, err := ethclient.Dial(wm.url)
	if err != nil {
		return err
	}
	wm.client = client
	return nil
}

func (wm *WebSocketManager) GetClient() *ethclient.Client {
	wm.reconnectMu.Lock()
	defer wm.reconnectMu.Unlock()
	return wm.client
}

func (wm *WebSocketManager) Reconnect() error {
	wm.reconnectMu.Lock()
	defer wm.reconnectMu.Unlock()

	if wm.reconnecting {
		return fmt.Errorf("reconnection already in progress")
	}

	wm.reconnecting = true
	defer func() { wm.reconnecting = false }()

	logger.Warn("WebSocket disconnected, attempting reconnection in %v...\n", wm.reconnectDelay)
	time.Sleep(wm.reconnectDelay)

	if wm.client != nil {
		wm.client.Close()
	}

	client, err := ethclient.Dial(wm.url)
	if err != nil {
		return err
	}

	wm.client = client
	logger.Info("✓ WebSocket reconnected successfully\n")
	return nil
}

func (wm *WebSocketManager) Close() {
	wm.reconnectMu.Lock()
	defer wm.reconnectMu.Unlock()
	if wm.client != nil {
		wm.client.Close()
	}
}

type DBWriteJob struct {
	Tx *db.Transaction
}

func StartDBWriterPool(workerCount int, jobChan <-chan DBWriteJob, database *db.Database, wg *sync.WaitGroup) {
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go dbWriterWorker(i+1, jobChan, database, wg)
	}
}

func dbWriterWorker(workerID int, jobChan <-chan DBWriteJob, database *db.Database, wg *sync.WaitGroup) {
	defer wg.Done()
	for job := range jobChan {
		if _, err := database.InsertTransaction(job.Tx); err != nil {
			logger.Warn("[DBWriter %d] Could not save transaction to DB: %v\n", workerID, err)
			continue
		}

		// Only dispatch a receipt job for transactions that were actually
		// submitted (have a hash). Failed submissions have no on-chain receipt.
		if job.Tx.TxHash == "" {
			logger.Debug("[DBWriter %d] Skipping receipt dispatch for failed submission (nonce %d)\n", workerID, job.Tx.Nonce)
			continue
		}

	}
}

func StartReceiptWorkerPool(workerCount int, jobChan chan ReceiptJob, wg *sync.WaitGroup, wsManager *WebSocketManager, database *db.Database, txSender *tx.TransactionSender) {
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go receiptWorker(i+1, jobChan, wg, wsManager, database, txSender)
	}
}

const maxReceiptRetries = 3

func receiptWorker(workerID int, jobChan chan ReceiptJob, wg *sync.WaitGroup, wsManager *WebSocketManager, database *db.Database, txSender *tx.TransactionSender) {
	defer wg.Done()

	jobsProcessed := 0
	for job := range jobChan {
		shouldRetry := processReceiptJob(workerID, txSender, job, wsManager, database)
		if shouldRetry {
			if job.RetryCount < maxReceiptRetries {
				job.RetryCount++
				logger.Warn("  [Worker %d] Re-queuing tx (nonce %d) for retry %d/%d\n", workerID, job.Nonce, job.RetryCount, maxReceiptRetries)

				// Non-blocking send to prevent deadlock
				select {
				case jobChan <- job:
					// Successfully re-queued
				default:
					// Channel full, mark as failed instead of hanging
					logger.Error("  [Worker %d] Channel full, marking tx (nonce %d) as failed\n", workerID, job.Nonce)
					database.UpdateTransactionStatus(job.TxHash, "failed", nil, 0, "", "retry queue full")
				}
			} else {
				logger.Error("  [Worker %d] Tx (nonce %d) exceeded max retries (%d), marking failed\n", workerID, job.Nonce, maxReceiptRetries)
				database.UpdateTransactionStatus(job.TxHash, "failed", nil, 0, "", "timeout after max retries")
			}
		} else {
			jobsProcessed++
		}
	}

	if txSender != nil {
		logger.Debug("[Worker %d] Closing RPC connection (%d jobs processed)\n", workerID, jobsProcessed)
		txSender.Close()
	}
}

func processReceiptJob(workerID int, txSender *tx.TransactionSender, job ReceiptJob, wsManager *WebSocketManager, database *db.Database) bool {
	// Add timeout to prevent indefinite hanging
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var wsClient *ethclient.Client
	if wsManager != nil {
		wsClient = wsManager.GetClient()
	}

	receipt, receiptErr := txSender.WaitForReceiptWithSharedWebSocket(ctx, wsClient, common.HexToHash(job.TxHash), 60*time.Second)

	if receiptErr != nil {
		if strings.Contains(receiptErr.Error(), "timeout waiting for transaction receipt") {
			logger.Warn("  [W%d] Tx (nonce %d): ⏱ timed out (retry %d/%d)\n", workerID, job.Nonce, job.RetryCount+1, maxReceiptRetries)
			return true
		}
		database.UpdateTransactionStatus(job.TxHash, "failed", nil, 0, "", receiptErr.Error())
		logger.Warn("  [W%d] Tx (nonce %d): ✗ error - %v\n", workerID, job.Nonce, receiptErr)
		return false
	}

	blockHeader, err := txSender.HeaderByHash(ctx, receipt.BlockHash)
	var confirmedAt time.Time
	if err != nil {
		logger.Warn("  [W%d] Could not fetch block header, using current time: %v\n", workerID, err)
		confirmedAt = time.Now()
	} else {
		confirmedAt = time.Unix(int64(blockHeader.Time), 0)
	}

	if confirmedAt.Before(job.StartTime) {
		logger.Warn("  [W%d] Block timestamp before submission time, adjusting\n", workerID)
		confirmedAt = job.StartTime.Add(1 * time.Second)
	}

	gasUsed := receipt.GasUsed
	effectiveGasPrice := ""
	if receipt.EffectiveGasPrice != nil {
		effectiveGasPrice = receipt.EffectiveGasPrice.String()
	}

	confirmationTime := confirmedAt.Sub(job.StartTime).Seconds()
	if receipt.Status == 1 {
		database.UpdateTransactionStatus(job.TxHash, "success", &confirmedAt, gasUsed, effectiveGasPrice, "")
		logger.Info("  [W%d] Tx (nonce %d): ✓ confirmed in %.2fs (gas: %d)\n", workerID, job.Nonce, confirmationTime, gasUsed)
	} else {
		database.UpdateTransactionStatus(job.TxHash, "failed", &confirmedAt, gasUsed, effectiveGasPrice, "transaction reverted")
		logger.Warn("  [W%d] Tx (nonce %d): ✗ reverted (transaction failed on-chain)\n", workerID, job.Nonce)
	}
	return false
}

// QueuePendingTransactionsForReceipt fetches pending transactions in batches and queues them for receipt processing
func QueuePendingTransactionsForReceipt(database *db.Database, receiptJobChan chan ReceiptJob) error {
	fmt.Println("\nProcessing pending transactions for receipt confirmation...")

	// Get total count of pending transactions
	pendingCount, err := database.GetPendingTransactionCount()
	if err != nil {
		logger.Error("Error getting pending transaction count: %v\n", err)
		return err
	}

	fmt.Printf("Found %d pending transactions to process\n", pendingCount)

	if pendingCount == 0 {
		fmt.Println("No pending transactions found to process")
		return nil
	}

	batchSize := 10000
	totalBatches := (pendingCount + batchSize - 1) / batchSize // Ceiling division

	fmt.Printf("Processing in %d batches of up to %d transactions each\n", totalBatches, batchSize)

	for batchNum := 0; batchNum < totalBatches; batchNum++ {
		offset := batchNum * batchSize
		fmt.Printf("Processing batch %d/%d (offset: %d)...\n", batchNum+1, totalBatches, offset)

		// Fetch batch of pending transactions
		pendingTxs, err := database.GetPendingTransactionsBatch(batchSize, offset)
		if err != nil {
			logger.Error("Error fetching pending transactions batch %d: %v\n", batchNum+1, err)
			continue
		}

		if len(pendingTxs) == 0 {
			fmt.Printf("No pending transactions in batch %d, stopping\n", batchNum+1)
			break
		}

		fmt.Printf("  Fetched %d transactions in batch %d\n", len(pendingTxs), batchNum+1)

		// Create receipt jobs and push to channel
		for _, tx := range pendingTxs {
			receiptJob := ReceiptJob{
				TxHash:     tx.TxHash,
				Nonce:      tx.Nonce,
				StartTime:  tx.SubmittedAt,
				RetryCount: 0,
			}
			receiptJobChan <- receiptJob
		}

		fmt.Printf("  ✓ Queued %d receipt jobs from batch %d\n", len(pendingTxs), batchNum+1)

	}

	fmt.Printf("✓ All %d pending transactions queued for receipt processing\n", pendingCount)
	return nil
}
