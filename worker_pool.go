package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go-tps/logger"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// ReceiptJob represents a receipt confirmation job
type ReceiptJob struct {
	TxHash     string
	Nonce      uint64
	StartTime  time.Time
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

// Close closes the WebSocket connection
func (wm *WebSocketManager) Close() {
	wm.reconnectMu.Lock()
	defer wm.reconnectMu.Unlock()
	if wm.client != nil {
		wm.client.Close()
	}
}

// DBWriteJob bundles a transaction insert with an optional follow-up receipt job.
// The receipt job (when non-nil) is dispatched to receiptJobChan only AFTER
// the INSERT succeeds, preventing the UPDATE-before-INSERT race condition.
type DBWriteJob struct {
	Tx *Transaction
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
			logger.Warn("[DBWriter %d] Could not save transaction to DB: %v\n", workerID, err)
			continue
		}
		// Only dispatch the receipt job AFTER the INSERT is confirmed

		receiptJobChan <- ReceiptJob{
			TxHash:     job.Tx.TxHash,
			Nonce:      job.Tx.Nonce,
			RetryCount: 0,
		}
		logger.Debug("[DBWriter %d] Inserted tx (nonce %d) and dispatched receipt job\n", workerID, job.Tx.Nonce)

	}
}

// startReceiptWorkerPool starts a pool of workers to process receipt confirmations
func startReceiptWorkerPool(workerCount int, jobChan chan ReceiptJob, wg *sync.WaitGroup, wsManager *WebSocketManager, db *Database, txSender *TransactionSender) {
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go receiptWorker(i+1, jobChan, wg, wsManager, db, txSender)
	}
}

const maxReceiptRetries = 3

// receiptWorker processes receipt confirmation jobs from the job channel
// with periodic connection refresh to prevent stale connections
func receiptWorker(workerID int, jobChan chan ReceiptJob, wg *sync.WaitGroup, wsManager *WebSocketManager, db *Database, txSender *TransactionSender) {
	defer wg.Done()

	// Process jobs from channel
	for job := range jobChan {
		shouldRetry := processReceiptJob(workerID, txSender, job, wsManager, db)
		if shouldRetry {
			if job.RetryCount < maxReceiptRetries {
				job.RetryCount++
				logger.Warn("  [Worker %d] Re-queuing tx (nonce %d) for retry %d/%d\n", workerID, job.Nonce, job.RetryCount, maxReceiptRetries)
				jobChan <- job
			} else {
				logger.Error("  [Worker %d] Tx (nonce %d) exceeded max retries (%d), marking failed\n", workerID, job.Nonce, maxReceiptRetries)
				db.UpdateTransactionStatus(job.TxHash, "failed", nil, 0, "", "timeout after max retries")
			}
		}
	}

	// Cleanup RPC connection
	if txSender != nil {
		logger.Debug("[Worker %d] Closing RPC connection (%d jobs processed)\n", workerID)
		txSender.Close()
	}
}

// processReceiptJob processes a single receipt confirmation job.
// Returns true if the job should be retried (i.e. it timed out).
func processReceiptJob(workerID int, txSender *TransactionSender, job ReceiptJob, wsManager *WebSocketManager, db *Database) bool {
	// Wait for receipt with timeout - use shared WebSocket if available
	ctx := context.Background()
	// get was client

	var wsClient *ethclient.Client
	if wsManager != nil {
		wsClient = wsManager.GetClient()
	}

	receipt, receiptErr := txSender.WaitForReceiptWithSharedWebSocket(ctx, wsClient, common.HexToHash(job.TxHash), 60*time.Second)

	if receiptErr != nil {
		// If this was a timeout, signal to the caller to re-queue the job
		if strings.Contains(receiptErr.Error(), "timeout waiting for transaction receipt") {
			logger.Warn("  [W%d] Tx (nonce %d): ⏱ timed out (retry %d/%d)\n", workerID, job.Nonce, job.RetryCount+1, maxReceiptRetries)
			return true
		}
		// For non-timeout errors, mark as failed immediately
		db.UpdateTransactionStatus(job.TxHash, "failed", nil, 0, "", receiptErr.Error())
		logger.Warn("  [W%d] Tx (nonce %d): ✗ error - %v\n", workerID, job.Nonce, receiptErr)
		return false
	} else {
		// Get block header to retrieve block timestamp
		blockHeader, err := txSender.client.HeaderByHash(ctx, receipt.BlockHash)
		var confirmedAt time.Time
		if err != nil {
			// Fallback to current time if block fetch fails
			logger.Warn("  [W%d] Could not fetch block header, using current time: %v\n", workerID, err)
			confirmedAt = time.Now()
		} else {
			// Use block creation time from the receipt's block
			confirmedAt = time.Unix(int64(blockHeader.Time), 0)
		}

		// Check for negative/zero block timestamp relative to submission time
		if confirmedAt.Before(job.StartTime) {
			logger.Warn("  [W%d] Block timestamp before submission time, adjusting\n", workerID)
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
			db.UpdateTransactionStatus(job.TxHash, "success", &confirmedAt, gasUsed, effectiveGasPrice, "")
			logger.Info("  [W%d] Tx (nonce %d): ✓ confirmed in %.2fs (gas: %d)\n", workerID, job.Nonce, confirmationTime, gasUsed)
		} else {
			db.UpdateTransactionStatus(job.TxHash, "failed", &confirmedAt, gasUsed, effectiveGasPrice, "transaction reverted")
			logger.Warn("  [W%d] Tx (nonce %d): ✗ reverted (transaction failed on-chain)\n", workerID, job.Nonce)
		}
		return false
	}
}
