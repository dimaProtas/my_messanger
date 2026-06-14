package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	ServerPort      string
	DatabaseURL     string
	RedisAddr       string
	KafkaBrokers    []string
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	AllowedOrigins  []string
}

func LoadConfig() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, loading from environment variables")
	}

	cfg := &Config{
		ServerPort:      getEnv("SERVER_PORT", "8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		RedisAddr:       getEnv("REDIS_ADDR", "localhost:6379"),
		KafkaBrokers:    getEnvAsSlice("KAFKA_BROKERS", "localhost:9092", ","),
		JWTSecret:       os.Getenv("JWT_SECRET"),
		AccessTokenTTL:  getEnvAsDuration("ACCESS_TOKEN_TTL_HOURS", 1),
		RefreshTokenTTL: getEnvAsDuration("REFRESH_TOKEN_TTL_HOURS", 24*7),
		AllowedOrigins:  getEnvAsSlice("ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:5173", ","),
	}

	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL not set in environment")
	}
	if cfg.JWTSecret == "" {
		log.Fatal("JWT_SECRET not set in environment")
	}

	return cfg
}

func getEnv(key string, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvAsSlice(key string, defaultValue string, sep string) []string {
	if value, exists := os.LookupEnv(key); exists {
		if value == "" {
			return []string{}
		}
		return splitAndTrim(value, sep)
	}
	return splitAndTrim(defaultValue, sep)
}

func splitAndTrim(s string, sep string) []string {
	var result []string
	parts := strings.Split(s, sep)
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			result = append(result, t)
		}
	}
	return result
}

func getEnvAsDuration(key string, defaultHours float64) time.Duration {
    valStr := getEnv(key, strconv.FormatFloat(defaultHours, 'f', -1, 64))
    valFloat, err := strconv.ParseFloat(valStr, 64)
    if err != nil {
        log.Printf("WARN: Could not parse %s as float, using default %f hours: %v", key, defaultHours, err)
        valFloat = defaultHours
    }
    return time.Duration(valFloat * float64(time.Hour))
}
