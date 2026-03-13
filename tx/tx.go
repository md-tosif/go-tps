package tx

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"go-tps/wallet"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type TransactionSender struct {
	client  *ethclient.Client
	chainID *big.Int
}

type TxRequest struct {
	Wallet    *wallet.Wallet
	ToAddress common.Address
	Value     *big.Int
	Nonce     uint64
	GasPrice  *big.Int
	GasLimit  uint64
	signedTx  *types.Transaction
}

type TxResult struct {
	TxHash        string
	Nonce         uint64
	Status        string
	SubmittedAt   time.Time
	ExecutionTime float64 // in milliseconds
	Error         error
}

func NewTransactionSender(rpcURL string) (*TransactionSender, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RPC: %w", err)
	}

	chainID, err := client.ChainID(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get chain ID: %w", err)
	}

	return &TransactionSender{
		client:  client,
		chainID: chainID,
	}, nil
}

func (ts *TransactionSender) GetNonce(ctx context.Context, address common.Address) (uint64, error) {
	nonce, err := ts.client.PendingNonceAt(ctx, address)
	if err != nil {
		return 0, fmt.Errorf("failed to get nonce: %w", err)
	}
	return nonce, nil
}

func (ts *TransactionSender) GetGasPrice(ctx context.Context) (*big.Int, error) {
	gasPrice, err := ts.client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get gas price: %w", err)
	}
	return gasPrice, nil
}

func (ts *TransactionSender) GetBalance(ctx context.Context, address common.Address) (*big.Int, error) {
	balance, err := ts.client.BalanceAt(ctx, address, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}
	return balance, nil
}

func (ts *TransactionSender) CreateTransaction(req *TxRequest) (*types.Transaction, error) {
	tx := types.NewTransaction(
		req.Nonce,
		req.ToAddress,
		req.Value,
		req.GasLimit,
		req.GasPrice,
		nil,
	)

	return tx, nil
}

func (ts *TransactionSender) SignTransaction(txn *types.Transaction, w *wallet.Wallet) (*types.Transaction, error) {
	signer := types.NewEIP155Signer(ts.chainID)
	signedTx, err := types.SignTx(txn, signer, w.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	return signedTx, nil
}

func (ts *TransactionSender) SendTransaction(ctx context.Context, signedTx *types.Transaction) (*TxResult, error) {
	startTime := time.Now()

	err := ts.client.SendTransaction(ctx, signedTx)

	executionTime := time.Since(startTime).Seconds() * 1000

	result := &TxResult{
		TxHash:        signedTx.Hash().Hex(),
		Nonce:         signedTx.Nonce(),
		SubmittedAt:   startTime,
		ExecutionTime: executionTime,
	}

	if err != nil {
		result.Status = "failed"
		result.Error = err
		return result, err
	}

	result.Status = "pending"
	return result, nil
}

func (ts *TransactionSender) CreateAndSendTransaction(ctx context.Context, req *TxRequest) (*TxResult, error) {
	result, err := ts.SendTransaction(ctx, req.signedTx)
	if err != nil {
		return result, err
	}

	return result, nil
}

func (ts *TransactionSender) SendMultipleTransactions(ctx context.Context, requests []*TxRequest) ([]*TxResult, error) {
	results := make([]*TxResult, 0, len(requests))

	for _, req := range requests {
		result, err := ts.CreateAndSendTransaction(ctx, req)
		if err != nil {
			results = append(results, result)
			continue
		}
		results = append(results, result)
	}

	return results, nil
}

func (ts *TransactionSender) Close() {
	if ts.client != nil {
		ts.client.Close()
	}
}

func (ts *TransactionSender) WaitForReceipt(ctx context.Context, txHash common.Hash, timeout time.Duration) (*types.Receipt, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for transaction receipt")
		case <-ticker.C:
			receipt, err := ts.client.TransactionReceipt(ctx, txHash)
			if err == nil {
				return receipt, nil
			}
			if err.Error() != "not found" {
				continue
			}
		}
	}
}

func (ts *TransactionSender) WaitForReceiptWithSharedWebSocket(ctx context.Context, wsClient *ethclient.Client, txHash common.Hash, timeout time.Duration) (*types.Receipt, error) {
	if wsClient == nil {
		return ts.WaitForReceipt(ctx, txHash, timeout)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Check immediately before subscribing — tx may already be mined.
	if receipt, err := ts.client.TransactionReceipt(ctx, txHash); err == nil {
		return receipt, nil
	}

	// SubscribeTransactionReceipts streams []*types.Receipt batches to the channel
	// whenever any of the watched transaction hashes get included in a block.
	query := &ethereum.TransactionReceiptsQuery{
		TransactionHashes: []common.Hash{txHash},
	}
	receiptCh := make(chan []*types.Receipt, 1)
	sub, err := wsClient.SubscribeTransactionReceipts(ctx, query, receiptCh)
	if err != nil {
		// WebSocket subscription failed; fall back to RPC polling.
		return ts.WaitForReceipt(ctx, txHash, timeout)
	}
	defer sub.Unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for transaction receipt")
		case err := <-sub.Err():
			// Subscription broken; fall back to polling.
			_ = err
			return ts.WaitForReceipt(ctx, txHash, timeout)
		case receipts := <-receiptCh:
			// A batch of receipts arrived; find the one matching our tx.
			for _, r := range receipts {
				if r.TxHash == txHash {
					return r, nil
				}
			}
		}
	}
}

func (ts *TransactionSender) PrepareBatchTransactions(ctx context.Context, w *wallet.Wallet, toAddress common.Address, value *big.Int, count int, gasPrice *big.Int) ([]*TxRequest, error) {

	startNonce := w.Nonce

	requests := make([]*TxRequest, 0, count)
	for i := 0; i < count; i++ {
		req := TxRequest{
			Wallet:    w,
			ToAddress: w.Address,
			Value:     value,
			Nonce:     startNonce + uint64(i),
			GasPrice:  gasPrice,
			GasLimit:  21000,
		}

		tx, err := ts.CreateTransaction(&req)
		if err != nil {
			return nil, fmt.Errorf("failed to create transaction: %w", err)
		}

		signedTx, err := ts.SignTransaction(tx, req.Wallet)
		if err != nil {
			return nil, fmt.Errorf("failed to sign transaction: %w", err)
		}

		req.signedTx = signedTx
		requests = append(requests, &req)

	}
	w.Nonce += uint64(count)
	return requests, nil
}

func (ts *TransactionSender) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	return ts.client.HeaderByHash(ctx, hash)
}
