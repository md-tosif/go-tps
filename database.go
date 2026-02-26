package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Transaction struct {
	ID            int64
	WalletAddress string
	TxHash        string
	Nonce         uint64
	ToAddress     string
	Value         string
	GasPrice      string
	GasLimit      uint64
	Status        string
	SubmittedAt   time.Time
	ConfirmedAt   *time.Time
	ExecutionTime float64 // in milliseconds
	Error         string
}

type Database struct {
	db *sql.DB
}

func NewDatabase(dbPath string) (*Database, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create tables
	if err := createTables(db); err != nil {
		db.Close()
		return nil, err
	}

	return &Database{db: db}, nil
}

func createTables(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS transactions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		wallet_address TEXT NOT NULL,
		tx_hash TEXT,
		nonce INTEGER NOT NULL,
		to_address TEXT NOT NULL,
		value TEXT NOT NULL,
		gas_price TEXT NOT NULL,
		gas_limit INTEGER NOT NULL,
		status TEXT NOT NULL,
		submitted_at TIMESTAMP NOT NULL,
		confirmed_at TIMESTAMP,
		execution_time REAL,
		error TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_wallet_address ON transactions(wallet_address);
	CREATE INDEX IF NOT EXISTS idx_tx_hash ON transactions(tx_hash);
	CREATE INDEX IF NOT EXISTS idx_status ON transactions(status);
	CREATE INDEX IF NOT EXISTS idx_submitted_at ON transactions(submitted_at);

	CREATE TABLE IF NOT EXISTS wallets (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		address TEXT NOT NULL UNIQUE,
		derivation_path TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL
	);
	`

	_, err := db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	return nil
}

func (d *Database) InsertTransaction(tx *Transaction) (int64, error) {
	query := `
		INSERT INTO transactions (
			wallet_address, tx_hash, nonce, to_address, value, 
			gas_price, gas_limit, status, submitted_at, confirmed_at, 
			execution_time, error
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := d.db.Exec(query,
		tx.WalletAddress,
		tx.TxHash,
		tx.Nonce,
		tx.ToAddress,
		tx.Value,
		tx.GasPrice,
		tx.GasLimit,
		tx.Status,
		tx.SubmittedAt,
		tx.ConfirmedAt,
		tx.ExecutionTime,
		tx.Error,
	)

	if err != nil {
		return 0, fmt.Errorf("failed to insert transaction: %w", err)
	}

	return result.LastInsertId()
}

func (d *Database) UpdateTransactionStatus(txHash, status string, confirmedAt *time.Time, executionTime float64, errMsg string) error {
	query := `
		UPDATE transactions 
		SET status = ?, confirmed_at = ?, execution_time = ?, error = ?
		WHERE tx_hash = ?
	`

	_, err := d.db.Exec(query, status, confirmedAt, executionTime, errMsg, txHash)
	if err != nil {
		return fmt.Errorf("failed to update transaction: %w", err)
	}

	return nil
}

func (d *Database) InsertWallet(address, derivationPath string) error {
	query := `
		INSERT INTO wallets (address, derivation_path, created_at)
		VALUES (?, ?, ?)
	`

	_, err := d.db.Exec(query, address, derivationPath, time.Now())
	if err != nil {
		return fmt.Errorf("failed to insert wallet: %w", err)
	}

	return nil
}

func (d *Database) GetTransactionStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total transactions
	var total int
	err := d.db.QueryRow("SELECT COUNT(*) FROM transactions").Scan(&total)
	if err != nil {
		return nil, err
	}
	stats["total_transactions"] = total

	// Average execution time
	var avgTime float64
	err = d.db.QueryRow("SELECT AVG(execution_time) FROM transactions WHERE execution_time > 0").Scan(&avgTime)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	stats["avg_execution_time_ms"] = avgTime

	// Success/failure counts
	var successful, failed, pending int
	d.db.QueryRow("SELECT COUNT(*) FROM transactions WHERE status = 'success'").Scan(&successful)
	d.db.QueryRow("SELECT COUNT(*) FROM transactions WHERE status = 'failed'").Scan(&failed)
	d.db.QueryRow("SELECT COUNT(*) FROM transactions WHERE status = 'pending'").Scan(&pending)

	stats["successful"] = successful
	stats["failed"] = failed
	stats["pending"] = pending

	return stats, nil
}

func (d *Database) Close() error {
	return d.db.Close()
}
