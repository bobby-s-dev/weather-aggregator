package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

type Config struct {
	Server struct {
		Port         string
		ReadTimeout  time.Duration
		WriteTimeout time.Duration
		LogLevel     string
	}
	
	WeatherAPI struct {
		OpenWeatherAPIKey string
		WeatherAPIKey     string
		OpenMeteoURL      string
	}
	
	Scheduler struct {
		FetchInterval time.Duration
		DefaultCities []string
	}
	
	Cache struct {
		Duration     time.Duration
		MaxSize      int
	}
	
	CircuitBreaker struct {
		Threshold int
		Timeout   time.Duration
	}
	
	Retry struct {
		MaxRetries int
		Delay      time.Duration
		Multiplier float64
	}
}

func LoadConfig() (*Config, error) {
	// Load .env file if exists
	if err := godotenv.Load(); err != nil {
		zap.L().Info("No .env file found, using environment variables")
	}

	cfg := &Config{}
	
	// Server configuration
	cfg.Server.Port = getEnv("FIBER_PORT", "8080")
	cfg.Server.ReadTimeout = parseDuration(getEnv("FIBER_READ_TIMEOUT", "10s"))
	cfg.Server.WriteTimeout = parseDuration(getEnv("FIBER_WRITE_TIMEOUT", "10s"))
	cfg.Server.LogLevel = getEnv("LOG_LEVEL", "info")
	
	// Weather API configuration
	cfg.WeatherAPI.OpenWeatherAPIKey = getEnv("OPENWEATHER_API_KEY", "")
	cfg.WeatherAPI.WeatherAPIKey = getEnv("WEATHERAPI_API_KEY", "")
	cfg.WeatherAPI.OpenMeteoURL = getEnv("OPENMETEO_URL", "https://api.open-meteo.com/v1")
	
	// Scheduler configuration
	cfg.Scheduler.FetchInterval = parseDuration(getEnv("FETCH_INTERVAL", "15m"))
	cities := getEnv("DEFAULT_CITIES", "Prague,London,NewYork")
	cfg.Scheduler.DefaultCities = strings.Split(cities, ",")
	
	// Cache configuration
	cfg.Cache.Duration = parseDuration(getEnv("CACHE_DURATION", "10m"))
	cfg.Cache.MaxSize = parseInt(getEnv("MAX_CACHE_SIZE", "1000"))
	
	// Circuit breaker configuration
	cfg.CircuitBreaker.Threshold = parseInt(getEnv("CIRCUIT_BREAKER_THRESHOLD", "3"))
	cfg.CircuitBreaker.Timeout = parseDuration(getEnv("CIRCUIT_BREAKER_TIMEOUT", "30s"))
	
	// Retry configuration
	cfg.Retry.MaxRetries = parseInt(getEnv("MAX_RETRIES", "3"))
	cfg.Retry.Delay = parseDuration(getEnv("RETRY_DELAY", "1s"))
	cfg.Retry.Multiplier = parseFloat(getEnv("RETRY_MULTIPLIER", "2"))
	
	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseDuration(value string) time.Duration {
	duration, err := time.ParseDuration(value)
	if err != nil {
		zap.L().Warn("Failed to parse duration", zap.String("value", value), zap.Error(err))
		return 0
	}
	return duration
}

func parseInt(value string) int {
	intValue, err := strconv.Atoi(value)
	if err != nil {
		zap.L().Warn("Failed to parse int", zap.String("value", value), zap.Error(err))
		return 0
	}
	return intValue
}

func parseFloat(value string) float64 {
	floatValue, err := strconv.ParseFloat(value, 64)
	if err != nil {
		zap.L().Warn("Failed to parse float", zap.String("value", value), zap.Error(err))
		return 0
	}
	return floatValue
}