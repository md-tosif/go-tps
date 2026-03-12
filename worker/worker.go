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

func StartDBWriterPool(workerCount int, jobChan <-chan DBWriteJob, receiptJobChan chan ReceiptJob, database *db.Database, wg *sync.WaitGroup) {
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go dbWriterWorker(i+1, jobChan, receiptJobChan, database, wg)
	}
}

func dbWriterWorker(workerID int, jobChan <-chan DBWriteJob, receiptJobChan chan ReceiptJob, database *db.Database, wg *sync.WaitGroup) {
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

		receiptJobChan <- ReceiptJob{
			TxHash:     job.Tx.TxHash,
			Nonce:      job.Tx.Nonce,
			RetryCount: 0,
		}
		logger.Debug("[DBWriter %d] Inserted tx (nonce %d) and dispatched receipt job\n", workerID, job.Tx.Nonce)
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
	ctx := context.Background()

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
