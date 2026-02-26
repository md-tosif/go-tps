package main

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type TransactionSender struct {
	client  *ethclient.Client
	chainID *big.Int
}

type TxRequest struct {
	Wallet    *Wallet
	ToAddress common.Address
	Value     *big.Int
	Nonce     uint64
	GasPrice  *big.Int
	GasLimit  uint64
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
		nil, // data
	)

	return tx, nil
}

func (ts *TransactionSender) SignTransaction(tx *types.Transaction, wallet *Wallet) (*types.Transaction, error) {
	signer := types.NewEIP155Signer(ts.chainID)
	signedTx, err := types.SignTx(tx, signer, wallet.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	return signedTx, nil
}

func (ts *TransactionSender) SendTransaction(ctx context.Context, signedTx *types.Transaction) (*TxResult, error) {
	startTime := time.Now()

	err := ts.client.SendTransaction(ctx, signedTx)

	executionTime := time.Since(startTime).Seconds() * 1000 // Convert to milliseconds

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
	// Create transaction
	tx, err := ts.CreateTransaction(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Sign transaction
	signedTx, err := ts.SignTransaction(tx, req.Wallet)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Send transaction
	result, err := ts.SendTransaction(ctx, signedTx)
	if err != nil {
		return result, err // Result contains error info
	}

	return result, nil
}

func (ts *TransactionSender) SendMultipleTransactions(ctx context.Context, requests []*TxRequest) ([]*TxResult, error) {
	results := make([]*TxResult, 0, len(requests))

	for _, req := range requests {
		result, err := ts.CreateAndSendTransaction(ctx, req)
		if err != nil {
			// Continue sending other transactions even if one fails
			results = append(results, result)
			continue
		}
		results = append(results, result)
	}

	return results, nil
}

func (ts *TransactionSender) Close() {
	ts.client.Close()
}

// WaitForReceipt waits for a transaction to be mined and returns the receipt
func (ts *TransactionSender) WaitForReceipt(ctx context.Context, txHash common.Hash, timeout time.Duration) (*types.Receipt, error) {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Poll for receipt
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
			// If error is not "not found", return it
			if err.Error() != "not found" {
				// Continue polling for "not found" errors
				continue
			}
		}
	}
}

// PrepareBatchTransactions prepares multiple transactions with precalculated nonces
func (ts *TransactionSender) PrepareBatchTransactions(
	ctx context.Context,
	wallet *Wallet,
	toAddress common.Address,
	value *big.Int,
	count int,
) ([]*TxRequest, error) {
	// Get starting nonce
	startNonce, err := ts.GetNonce(ctx, wallet.Address)
	if err != nil {
		return nil, err
	}

	// Get gas price
	gasPrice, err := ts.GetGasPrice(ctx)
	if err != nil {
		return nil, err
	}

	// Prepare transactions with precalculated nonces
	requests := make([]*TxRequest, 0, count)
	for i := 0; i < count; i++ {
		req := &TxRequest{
			Wallet:    wallet,
			ToAddress: toAddress,
			Value:     value,
			Nonce:     startNonce + uint64(i),
			GasPrice:  gasPrice,
			GasLimit:  21000, // Standard ETH transfer
		}
		requests = append(requests, req)
	}

	return requests, nil
}
