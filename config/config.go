package config

import (
	"fmt"
	"os"
	"strings"
)

const (
	DefaultRPCURL             = "http://localhost:8545"
	DefaultWSURL              = "http://localhost:8546" // Empty = no WebSocket, will use RPC polling
	DefaultDBPath             = "./transactions.db"
	DefaultWalletCount        = 10
	DefaultTxPerWallet        = 10
	DefaultValueWei           = "1000000000000000" // 0.001 ETH
	DefaultToAddress          = "0x0000000000000000000000000000000000000001"
	DefaultRunDurationMinutes = 0            // 0 = run once, >0 = loop for duration
	DefaultDBWorkers          = 4            // DB writer workers
	DefaultReceiptWorkers     = 4            // Receipt confirmation workers
	DefaultLogLevel           = "DEBUG"      // DEBUG, INFO, WARN, ERROR
	DefaultAutomatedMode      = false        // true = skip user confirmation
	DefaultContextTimeout     = 30           // seconds for RPC calls
	DefaultDBRetentionDays    = 30           // cleanup records older than this
	DefaultWSReconnectDelay   = 5            // seconds before reconnecting WebSocket
	DefaultDBBufferSize       = 500          // DB channel buffer size (0 = auto-calculate from WalletCount * TxPerWallet)
	DefaultReceiptBufferSize  = 1000         // Receipt channel buffer size (0 = auto-calculate)
	DefaultDBMaxOpenConns     = 15           // max open DB connections
	DefaultDBMaxIdleConns     = 5            // max idle DB connections
	DefaultSleepMinutes       = 0            // minutes to sleep before submitting transactions
	DefaultGasLimit           = 25000        // gas limit for transactions (increased from 21000 to prevent out of gas)
	DefaultMinGasPrice        = "2000000000" // minimum gas price in wei (2 gwei)
)

type Config struct {
	RPCURL             string
	WSURL              string
	DBPath             string
	Mnemonic           string
	WalletCount        int
	TxPerWallet        int
	ValueWei           string
	ToAddress          string
	RunDurationMinutes int
	DBWorkers          int // Number of DB writer workers
	ReceiptWorkers     int // Number of receipt confirmation workers
	LogLevel           string
	AutomatedMode      bool   // Skip user confirmation if true
	ContextTimeout     int    // Timeout for RPC calls in seconds
	WSReconnectDelay   int    // Seconds before reconnecting WebSocket
	DBBufferSize       int    // DB channel buffer size (0 = auto-calculate)
	ReceiptBufferSize  int    // Receipt channel buffer size (0 = auto-calculate)
	DBMaxOpenConns     int    // Max open SQLite connections
	DBMaxIdleConns     int    // Max idle SQLite connections
	SleepMinutes       int    // Minutes to sleep before submitting transactions
	GasLimit           uint64 // Gas limit for transactions
	MinGasPrice        string // Minimum gas price in wei
}

func LoadConfig() *Config {
	// Load from environment variables or use defaults
	config := &Config{
		RPCURL:             getEnv("RPC_URL", DefaultRPCURL),
		WSURL:              getEnv("WS_URL", DefaultWSURL),
		DBPath:             getEnv("DB_PATH", DefaultDBPath),
		Mnemonic:           getEnv("MNEMONIC", ""),
		WalletCount:        getEnvInt("WALLET_COUNT", DefaultWalletCount),
		TxPerWallet:        getEnvInt("TX_PER_WALLET", DefaultTxPerWallet),
		ValueWei:           getEnv("VALUE_WEI", DefaultValueWei),
		ToAddress:          getEnv("TO_ADDRESS", DefaultToAddress),
		RunDurationMinutes: getEnvInt("RUN_DURATION_MINUTES", DefaultRunDurationMinutes),
		DBWorkers:          getEnvInt("DB_WORKERS", DefaultDBWorkers),
		ReceiptWorkers:     getEnvInt("RECEIPT_WORKERS", DefaultReceiptWorkers),
		LogLevel:           getEnv("LOG_LEVEL", DefaultLogLevel),
		AutomatedMode:      getEnvBool("AUTOMATED_MODE", DefaultAutomatedMode),
		ContextTimeout:     getEnvInt("CONTEXT_TIMEOUT", DefaultContextTimeout),
		WSReconnectDelay:   getEnvInt("WS_RECONNECT_DELAY", DefaultWSReconnectDelay),
		DBBufferSize:       getEnvInt("DB_BUFFER_SIZE", DefaultDBBufferSize),
		ReceiptBufferSize:  getEnvInt("RECEIPT_BUFFER_SIZE", DefaultReceiptBufferSize),
		DBMaxOpenConns:     getEnvInt("DB_MAX_OPEN_CONNS", DefaultDBMaxOpenConns),
		DBMaxIdleConns:     getEnvInt("DB_MAX_IDLE_CONNS", DefaultDBMaxIdleConns),
		SleepMinutes:       getEnvInt("SLEEP_MINUTES", DefaultSleepMinutes),
		GasLimit:           getEnvUint64("GAS_LIMIT", DefaultGasLimit),
		MinGasPrice:        getEnv("MIN_GAS_PRICE", DefaultMinGasPrice),
	}

	return config
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	var intValue int
	_, err := fmt.Sscanf(value, "%d", &intValue)
	if err != nil {
		fmt.Printf("Warning: Invalid integer value for %s: '%s', using default: %d\n", key, value, defaultValue)
		return defaultValue
	}
	return intValue
}

func getEnvUint64(key string, defaultValue uint64) uint64 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	var uint64Value uint64
	_, err := fmt.Sscanf(value, "%d", &uint64Value)
	if err != nil {
		fmt.Printf("Warning: Invalid uint64 value for %s: '%s', using default: %d\n", key, value, defaultValue)
		return defaultValue
	}
	return uint64Value
}

func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	switch strings.ToLower(value) {
	case "true", "1", "yes", "y":
		return true
	case "false", "0", "no", "n":
		return false
	default:
		fmt.Printf("Warning: Invalid boolean value for %s: '%s', using default: %v\n", key, value, defaultValue)
		return defaultValue
	}
}
