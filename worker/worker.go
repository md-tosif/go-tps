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

				// Add exponential backoff delay before retry to avoid overwhelming the network
				retryDelay := time.Duration(job.RetryCount*job.RetryCount) * 30 * time.Second // 30s, 120s, 270s
				logger.Debug("  [Worker %d] Waiting %v before retry for tx (nonce %d)\n", workerID, retryDelay, job.Nonce)
				time.Sleep(retryDelay)

				// Block until we can re-queue - no new transactions being added during receipt processing
				jobChan <- job
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

// QueuePendingTransactionsForReceipt fetches pending transactions and queues them for receipt processing
// Processes in controlled batches of 1000, waiting for completion before queuing more
func QueuePendingTransactionsForReceipt(database *db.Database, receiptJobChan chan ReceiptJob) error {
	fmt.Println("\nProcessing pending transactions for receipt confirmation...")

	const batchSize = 1000
	lastTxnID := int64(0)
	totalProcessed := 0

	for {
		// Get the next batch of pending transactions using cursor-based pagination
		transactions, err := database.GetPendingTransactionsBatchCursor(lastTxnID, batchSize)
		if err != nil {
			return fmt.Errorf("failed to fetch pending transactions: %w", err)
		}

		// If no transactions returned, we're done
		if len(transactions) == 0 {
			break
		}

		// Queue all jobs from this batch
		for _, tx := range transactions {
			job := ReceiptJob{
				TxHash:     tx.TxHash,
				Nonce:      tx.Nonce,
				StartTime:  tx.SubmittedAt,
				RetryCount: 0,
			}

			select {
			case receiptJobChan <- job:
				// Job queued successfully
			default:
				// Channel is full - this shouldn't happen with proper sizing
				logger.Warn("Receipt job channel is full, this may cause delays")
				receiptJobChan <- job // Block until we can send
			}

			// Update the cursor to the last processed transaction ID
			lastTxnID = tx.ID
		}

		totalProcessed += len(transactions)
		logger.Info("Queued batch of %d pending transactions for receipt processing (total: %d)", len(transactions), totalProcessed)

		// If we got less than batchSize, we've processed all remaining transactions
		if len(transactions) < batchSize {
			break
		}
	}

	logger.Info("✓ Completed queuing %d pending transactions for receipt processing", totalProcessed)
	return nil
}
