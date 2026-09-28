package config

import (
	"os"
	"strconv"
	"time"
)

const (
	defaultMaxUploadMB      = 50
	defaultMaxSheets        = 50
	defaultMaxCellsPerSheet = 200000
	defaultTimeoutSeconds   = 10
	defaultIncludeHidden    = false
	defaultAddr             = ":8080"
	defaultEnableDebugVars  = false
)

// Config defines runtime limits and behavior.
type Config struct {
	Addr                string
	MaxUploadBytes      int64
	MaxSheets           int
	MaxCellsPerSheet    int
	ConversionTimeout   time.Duration
	IncludeHiddenSheets bool
	EnableDebugVars     bool
}

// Load reads environment variables and returns a populated config.
func Load() Config {
	maxUploadMB := getEnvInt("MAX_UPLOAD_MB", defaultMaxUploadMB)
	maxSheets := getEnvInt("MAX_SHEETS", defaultMaxSheets)
	maxCells := getEnvInt("MAX_CELLS_PER_SHEET", defaultMaxCellsPerSheet)
	timeoutSeconds := getEnvInt("CONVERSION_TIMEOUT_SECONDS", defaultTimeoutSeconds)
	includeHidden := getEnvBool("INCLUDE_HIDDEN_SHEETS", defaultIncludeHidden)
	addr := getEnvString("ADDR", defaultAddr)
	enableDebug := getEnvBool("ENABLE_DEBUG_VARS", defaultEnableDebugVars)

	return Config{
		Addr:                addr,
		MaxUploadBytes:      int64(maxUploadMB) << 20,
		MaxSheets:           maxSheets,
		MaxCellsPerSheet:    maxCells,
		ConversionTimeout:   time.Duration(timeoutSeconds) * time.Second,
		IncludeHiddenSheets: includeHidden,
		EnableDebugVars:     enableDebug,
	}
}

func getEnvString(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
