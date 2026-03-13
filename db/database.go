package db

import (
	"database/sql"
	"fmt"
	"time"

	"go-tps/logger"

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

func NewDatabase(dbPath string, maxOpenConns, maxIdleConns int) (*Database, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_cache_size=-64000", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(0)

	if err := createTables(db); err != nil {
		db.Close()
		return nil, err
	}

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

func optimizeDatabase(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=-64000",
		"PRAGMA temp_store=MEMORY",
		"PRAGMA mmap_size=268435456",
		"PRAGMA page_size=4096",
		"PRAGMA auto_vacuum=INCREMENTAL",
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

	logger.Debug("[DB] INSERT tx_hash=%s status=%s nonce=%d wallet=%s\n", tx.TxHash, tx.Status, tx.Nonce, tx.WalletAddress)

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
		logger.Error("[DB] INSERT FAILED tx_hash=%s error=%v\n", tx.TxHash, err)
		return 0, fmt.Errorf("failed to insert transaction: %w", err)
	}

	id, err := result.LastInsertId()
	logger.Debug("[DB] INSERT OK tx_hash=%s id=%d\n", tx.TxHash, id)
	return id, err
}

func (d *Database) UpdateTransactionStatus(txHash, status string, confirmedAt *time.Time, gasUsed uint64, effectiveGasPrice string, errMsg string) error {
	logger.Debug("[DB] UPDATE tx_hash=%s status=%s gas_used=%d err=%q\n", txHash, status, gasUsed, errMsg)

	query := `
		UPDATE transactions
		SET status = ?, confirmed_at = ?, gas_used = ?, effective_gas_price = ?, error = ?
		WHERE tx_hash = ?
	`

	_, err := d.db.Exec(query, status, confirmedAt, gasUsed, effectiveGasPrice, errMsg, txHash)
	if err != nil {
		logger.Error("[DB] UPDATE FAILED tx_hash=%s error=%v\n", txHash, err)
		return fmt.Errorf("failed to update transaction: %w", err)
	}

	logger.Debug("[DB] UPDATE OK tx_hash=%s\n", txHash)
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

func (d *Database) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// GetPendingTransactionsBatch fetches pending transactions in batches
func (d *Database) GetPendingTransactionsBatch(limit, offset int) ([]*Transaction, error) {
	query := `
		SELECT id, batch_number, wallet_address, tx_hash, nonce, to_address, 
		       value, gas_price, gas_limit, gas_used, effective_gas_price, 
		       status, submitted_at, confirmed_at, execution_time, error
		FROM transactions 
		WHERE status = 'pending' AND tx_hash IS NOT NULL AND tx_hash != ''
		ORDER BY submitted_at ASC
		LIMIT ? OFFSET ?
	`

	rows, err := d.db.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending transactions: %w", err)
	}
	defer rows.Close()

	var transactions []*Transaction
	for rows.Next() {
		tx := &Transaction{}
		err := rows.Scan(
			&tx.ID, &tx.BatchNumber, &tx.WalletAddress, &tx.TxHash, &tx.Nonce,
			&tx.ToAddress, &tx.Value, &tx.GasPrice, &tx.GasLimit, &tx.GasUsed,
			&tx.EffectiveGasPrice, &tx.Status, &tx.SubmittedAt, &tx.ConfirmedAt,
			&tx.ExecutionTime, &tx.Error,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}
		transactions = append(transactions, tx)
	}

	return transactions, nil
}

// GetPendingTransactionCount returns the total count of pending transactions
func (d *Database) GetPendingTransactionCount() (int, error) {
	query := `SELECT COUNT(*) FROM transactions WHERE status = 'pending' AND tx_hash IS NOT NULL AND tx_hash != ''`

	var count int
	err := d.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count pending transactions: %w", err)
	}

	return count, nil
}

// GetPendingTransactionsBatchCursor fetches pending transactions using cursor-based pagination
// This is more efficient than OFFSET/LIMIT for large datasets and avoids missing records
func (d *Database) GetPendingTransactionsBatchCursor(lastID int64, limit int) ([]*Transaction, error) {
	query := `
		SELECT id, batch_number, wallet_address, tx_hash, nonce, to_address, 
		       value, gas_price, gas_limit, gas_used, effective_gas_price, 
		       status, submitted_at, confirmed_at, execution_time, error
		FROM transactions 
		WHERE status IN ('pending', 'failed') AND tx_hash IS NOT NULL AND tx_hash != ''
		AND id > ?
		ORDER BY id ASC
		LIMIT ?
	`

	rows, err := d.db.Query(query, lastID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending transactions: %w", err)
	}
	defer rows.Close()

	var transactions []*Transaction
	for rows.Next() {
		tx := &Transaction{}
		err := rows.Scan(
			&tx.ID, &tx.BatchNumber, &tx.WalletAddress, &tx.TxHash, &tx.Nonce,
			&tx.ToAddress, &tx.Value, &tx.GasPrice, &tx.GasLimit, &tx.GasUsed,
			&tx.EffectiveGasPrice, &tx.Status, &tx.SubmittedAt, &tx.ConfirmedAt,
			&tx.ExecutionTime, &tx.Error,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}
		transactions = append(transactions, tx)
	}

	return transactions, nil
}
