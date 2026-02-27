package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Transaction struct {
	ID            int64
	BatchNumber   string
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
		batch_number TEXT NOT NULL,
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

	CREATE INDEX IF NOT EXISTS idx_batch_number ON transactions(batch_number);
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
			batch_number, wallet_address, tx_hash, nonce, to_address, value, 
			gas_price, gas_limit, status, submitted_at, confirmed_at, 
			execution_time, error
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := d.db.Exec(query,
		tx.BatchNumber,
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

	// Calculate TPS based on submission times
	tpsData, err := d.CalculateTPS()
	if err == nil {
		for key, value := range tpsData {
			stats[key] = value
		}
	}

	return stats, nil
}

func (d *Database) CalculateTPS() (map[string]interface{}, error) {
	tpsStats := make(map[string]interface{})

	// Get time range of all transactions
	var minTime, maxTime sql.NullTime
	var txCount int

	err := d.db.QueryRow(`
		SELECT 
			MIN(submitted_at) as min_time,
			MAX(submitted_at) as max_time,
			COUNT(*) as tx_count
		FROM transactions
		WHERE status = 'success'
	`).Scan(&minTime, &maxTime, &txCount)

	if err != nil || !minTime.Valid || !maxTime.Valid || txCount == 0 {
		tpsStats["tps_submission"] = 0.0
		tpsStats["tps_confirmation"] = 0.0
		return tpsStats, nil
	}

	// Calculate TPS based on submission time window
	submissionDuration := maxTime.Time.Sub(minTime.Time).Seconds()
	if submissionDuration > 0 {
		tpsStats["tps_submission"] = float64(txCount) / submissionDuration
		tpsStats["submission_duration_seconds"] = submissionDuration
	} else {
		tpsStats["tps_submission"] = 0.0
		tpsStats["submission_duration_seconds"] = 0.0
	}

	// Calculate TPS based on confirmation time window
	var minConfirmed, maxConfirmed sql.NullTime
	var confirmedCount int

	err = d.db.QueryRow(`
		SELECT 
			MIN(confirmed_at) as min_confirmed,
			MAX(confirmed_at) as max_confirmed,
			COUNT(*) as confirmed_count
		FROM transactions
		WHERE status = 'success' AND confirmed_at IS NOT NULL
	`).Scan(&minConfirmed, &maxConfirmed, &confirmedCount)

	if err == nil && minConfirmed.Valid && maxConfirmed.Valid && confirmedCount > 0 {
		confirmationDuration := maxConfirmed.Time.Sub(minConfirmed.Time).Seconds()
		if confirmationDuration > 0 {
			tpsStats["tps_confirmation"] = float64(confirmedCount) / confirmationDuration
			tpsStats["confirmation_duration_seconds"] = confirmationDuration
		} else {
			tpsStats["tps_confirmation"] = float64(confirmedCount)
			tpsStats["confirmation_duration_seconds"] = 0.0
		}
	} else {
		tpsStats["tps_confirmation"] = 0.0
		tpsStats["confirmation_duration_seconds"] = 0.0
	}

	return tpsStats, nil
}

func (d *Database) GetBatchStats(batchNumber string) (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	stats["batch_number"] = batchNumber

	// Total transactions in this batch
	var total int
	err := d.db.QueryRow("SELECT COUNT(*) FROM transactions WHERE batch_number = ?", batchNumber).Scan(&total)
	if err != nil {
		return nil, err
	}
	stats["total_transactions"] = total

	// Success/failure counts
	var successful, failed, pending int
	d.db.QueryRow("SELECT COUNT(*) FROM transactions WHERE batch_number = ? AND status = 'success'", batchNumber).Scan(&successful)
	d.db.QueryRow("SELECT COUNT(*) FROM transactions WHERE batch_number = ? AND status = 'failed'", batchNumber).Scan(&failed)
	d.db.QueryRow("SELECT COUNT(*) FROM transactions WHERE batch_number = ? AND status = 'pending'", batchNumber).Scan(&pending)

	stats["successful"] = successful
	stats["failed"] = failed
	stats["pending"] = pending

	// Average execution time for this batch
	var avgTime sql.NullFloat64
	err = d.db.QueryRow("SELECT AVG(execution_time) FROM transactions WHERE batch_number = ? AND execution_time > 0", batchNumber).Scan(&avgTime)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if avgTime.Valid {
		stats["avg_execution_time_ms"] = avgTime.Float64
	} else {
		stats["avg_execution_time_ms"] = 0.0
	}

	// Time range for this batch
	var minTime, maxTime sql.NullTime
	err = d.db.QueryRow(`
		SELECT MIN(submitted_at), MAX(submitted_at) 
		FROM transactions 
		WHERE batch_number = ?
	`, batchNumber).Scan(&minTime, &maxTime)

	if err == nil && minTime.Valid && maxTime.Valid {
		stats["started_at"] = minTime.Time.Format(time.RFC3339)
		stats["completed_at"] = maxTime.Time.Format(time.RFC3339)
		duration := maxTime.Time.Sub(minTime.Time).Seconds()
		stats["duration_seconds"] = duration
		if duration > 0 && total > 0 {
			stats["tps"] = float64(total) / duration
		}
	}

	return stats, nil
}

func (d *Database) ListBatches() ([]string, error) {
	rows, err := d.db.Query(`
		SELECT DISTINCT batch_number 
		FROM transactions 
		ORDER BY batch_number DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var batches []string
	for rows.Next() {
		var batch string
		if err := rows.Scan(&batch); err != nil {
			return nil, err
		}
		batches = append(batches, batch)
	}

	return batches, rows.Err()
}

// GetFailedTransactions retrieves failed transactions for a specific batch
func (d *Database) GetFailedTransactions(batchNumber string, limit int) ([]map[string]string, error) {
	query := `
		SELECT wallet_address, nonce, error, tx_hash
		FROM transactions 
		WHERE batch_number = ? AND status = 'failed' AND error IS NOT NULL AND error != ''
		ORDER BY id
	`

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := d.db.Query(query, batchNumber)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var failures []map[string]string
	for rows.Next() {
		var walletAddr, nonce, errMsg, txHash string
		if err := rows.Scan(&walletAddr, &nonce, &errMsg, &txHash); err != nil {
			return nil, err
		}

		failures = append(failures, map[string]string{
			"wallet_address": walletAddr,
			"nonce":          nonce,
			"error":          errMsg,
			"tx_hash":        txHash,
		})
	}

	return failures, rows.Err()
}

func (d *Database) Close() error {
	return d.db.Close()
}
