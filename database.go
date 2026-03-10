package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Transaction struct {
	ID                int64
	BatchNumber       string
	WalletAddress     string
	TxHash            string
	Nonce             uint64
	ToAddress         string
	Value             string
	GasPrice          string
	GasLimit          uint64
	GasUsed           uint64
	EffectiveGasPrice string
	Status            string
	SubmittedAt       time.Time
	ConfirmedAt       *time.Time
	ExecutionTime     float64 // in milliseconds
	Error             string
}

type Database struct {
	db *sql.DB
}

func NewDatabase(dbPath string) (*Database, error) {
	// Use SQLite with WAL mode for better concurrency and performance
	// Also set busy_timeout to handle lock contention
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_cache_size=-64000", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings for better performance
	db.SetMaxOpenConns(25)   // Limit concurrent connections
	db.SetMaxIdleConns(5)    // Keep idle connections
	db.SetConnMaxLifetime(0) // No limit on connection lifetime

	// Create tables
	if err := createTables(db); err != nil {
		db.Close()
		return nil, err
	}

	// Optimize SQLite settings
	if err := optimizeDatabase(db); err != nil {
		return nil, fmt.Errorf("failed to optimize database: %w", err)
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
		gas_used INTEGER,
		effective_gas_price TEXT,
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

// optimizeDatabase sets pragmas for optimal SQLite performance
func optimizeDatabase(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",        // Write-Ahead Logging for concurrency
		"PRAGMA synchronous=NORMAL",      // Faster writes, still safe
		"PRAGMA cache_size=-64000",       // 64MB cache
		"PRAGMA temp_store=MEMORY",       // Use memory for temp tables
		"PRAGMA mmap_size=268435456",     // 256MB memory-mapped I/O
		"PRAGMA page_size=4096",          // 4KB page size
		"PRAGMA auto_vacuum=INCREMENTAL", // Incremental vacuum
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return fmt.Errorf("failed to execute %s: %w", pragma, err)
		}
	}

	return nil
}

func (d *Database) InsertTransaction(tx *Transaction) (int64, error) {

	query := `
		INSERT INTO transactions (
			batch_number, wallet_address, tx_hash, nonce, to_address, value, 
			gas_price, gas_limit, gas_used, effective_gas_price, status, submitted_at, confirmed_at, 
			execution_time, error
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	logDebug("[DB] INSERT tx_hash=%s status=%s nonce=%d wallet=%s\n", tx.TxHash, tx.Status, tx.Nonce, tx.WalletAddress)

	result, err := d.db.Exec(query,
		tx.BatchNumber,
		tx.WalletAddress,
		tx.TxHash,
		tx.Nonce,
		tx.ToAddress,
		tx.Value,
		tx.GasPrice,
		tx.GasLimit,
		tx.GasUsed,
		tx.EffectiveGasPrice,
		tx.Status,
		tx.SubmittedAt,
		tx.ConfirmedAt,
		tx.ExecutionTime,
		tx.Error,
	)

	if err != nil {
		logError("[DB] INSERT FAILED tx_hash=%s error=%v\n", tx.TxHash, err)
		return 0, fmt.Errorf("failed to insert transaction: %w", err)
	}

	id, err := result.LastInsertId()
	logDebug("[DB] INSERT OK tx_hash=%s id=%d\n", tx.TxHash, id)
	return id, err
}

func (d *Database) UpdateTransactionStatus(txHash, status string, confirmedAt *time.Time, gasUsed uint64, effectiveGasPrice string, errMsg string) error {

	logDebug("[DB] UPDATE tx_hash=%s status=%s gas_used=%d err=%q\n", txHash, status, gasUsed, errMsg)

	query := `
		UPDATE transactions 
		SET status = ?, confirmed_at = ?, gas_used = ?, effective_gas_price = ?, error = ?
		WHERE tx_hash = ?
	`

	_, err := d.db.Exec(query, status, confirmedAt, gasUsed, effectiveGasPrice, errMsg, txHash)
	if err != nil {
		logError("[DB] UPDATE FAILED tx_hash=%s error=%v\n", txHash, err)
		return fmt.Errorf("failed to update transaction: %w", err)
	}

	logDebug("[DB] UPDATE OK tx_hash=%s\n", txHash)
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

// GetPendingCount returns the number of transactions still in "pending" status.
func (d *Database) GetPendingCount() (int, error) {
	var count int
	err := d.db.QueryRow("SELECT COUNT(*) FROM transactions WHERE status = 'pending'").Scan(&count)
	return count, err
}

// GetPendingTransactions returns all transactions still in "pending" status.
func (d *Database) GetPendingTransactions() ([]*Transaction, error) {
	rows, err := d.db.Query(`
		SELECT tx_hash, nonce, submitted_at
		FROM transactions
		WHERE status = 'pending' AND tx_hash IS NOT NULL AND tx_hash != ''
		ORDER BY submitted_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txns []*Transaction
	for rows.Next() {
		tx := &Transaction{}
		if err := rows.Scan(&tx.TxHash, &tx.Nonce, &tx.SubmittedAt); err != nil {
			return nil, err
		}
		txns = append(txns, tx)
	}
	return txns, rows.Err()
}

// CleanupOldRecords removes transactions older than the specified number of days
// Returns the number of records deleted
func (d *Database) CleanupOldRecords(retentionDays int) (int64, error) {

	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	query := `
DELETE FROM transactions 
WHERE submitted_at < ?
`

	result, err := d.db.Exec(query, cutoffDate)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old records: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	// Run incremental vacuum to reclaim space
	if rowsAffected > 0 {
		_, _ = d.db.Exec("PRAGMA incremental_vacuum")
		fmt.Printf("✓ Cleaned up %d old records (retention: %d days)\n", rowsAffected, retentionDays)
	}

	return rowsAffected, nil
}

// GetDatabaseSize returns the size of the database in bytes
func (d *Database) GetDatabaseSize() (map[string]interface{}, error) {
	var pageCount, pageSize int64

	err := d.db.QueryRow("PRAGMA page_count").Scan(&pageCount)
	if err != nil {
		return nil, err
	}

	err = d.db.QueryRow("PRAGMA page_size").Scan(&pageSize)
	if err != nil {
		return nil, err
	}

	sizeBytes := pageCount * pageSize
	sizeMB := float64(sizeBytes) / (1024 * 1024)

	return map[string]interface{}{
		"size_bytes": sizeBytes,
		"size_mb":    fmt.Sprintf("%.2f", sizeMB),
		"page_count": pageCount,
		"page_size":  pageSize,
	}, nil
}
