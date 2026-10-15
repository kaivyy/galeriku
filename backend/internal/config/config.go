package config

import (
	"os"
)

type Config struct {
	AppEnv     string
	AppPort    string
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string
	RedisHost  string
	RedisPort  string
	RabbitHost string
	RabbitPort string
	QdrantHost string
	QdrantPort string
	S3Endpoint  string
	S3AccessKey string
	S3SecretKey string
	S3UseSSL    bool
	JWTSecret   string
	OTPSecret   string
	BankKey     string
}

func LoadConfig() *Config {
	return &Config{
		AppEnv:     getEnv("APP_ENV", "development"),
		AppPort:    getEnv("APP_PORT", "8080"),
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "galeriku"),
		DBPassword: getEnv("DB_PASSWORD", "galeriku_secret_pass"),
		DBName:     getEnv("DB_NAME", "galeriku_db"),
		DBSSLMode:  getEnv("DB_SSLMODE", "disable"),
		RedisHost:  getEnv("REDIS_HOST", "localhost"),
		RedisPort:  getEnv("REDIS_PORT", "6379"),
		RabbitHost: getEnv("RABBITMQ_HOST", "localhost"),
		RabbitPort: getEnv("RABBITMQ_PORT", "5672"),
		QdrantHost: getEnv("QDRANT_HOST", "localhost"),
		QdrantPort: getEnv("QDRANT_PORT", "6333"),
		S3Endpoint:  getEnv("S3_ENDPOINT", "localhost:9010"),
		S3AccessKey: getEnv("S3_ACCESS_KEY", "galeriku_admin"),
		S3SecretKey: getEnv("S3_SECRET_KEY", "galeriku_minio_secret"),
		S3UseSSL:    getEnv("S3_USE_SSL", "false") == "true",
		JWTSecret:   getEnv("JWT_SECRET", "default_dev_secret_change_in_production"),
		OTPSecret:   getEnv("OTP_SECRET_KEY", "default_otp_dev_secret_key_min_32_bytes_long!"),
		BankKey:     getEnv("BANK_CIPHER_KEY", "default_bank_cipher_key_32byte!!"),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
