package config

import "os"

type Config struct {
	Port               string
	DatabaseURL        string
	JWTSecret          string
	RabbitMQURL        string
	MinIOEndpoint      string
	MinIOAccessKey     string
	MinIOSecretKey     string
	MinIOVideosBucket  string
	MinIOResultsBucket string
}

func Load() Config {
	return Config{
		Port:               getEnv("PORT", "8080"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		RabbitMQURL:        os.Getenv("RABBITMQ_URL"),
		MinIOEndpoint:      os.Getenv("MINIO_ENDPOINT"),
		MinIOAccessKey:     os.Getenv("MINIO_ACCESS_KEY"),
		MinIOSecretKey:     os.Getenv("MINIO_SECRET_KEY"),
		MinIOVideosBucket:  getEnv("MINIO_BUCKET_VIDEOS", "videos"),
		MinIOResultsBucket: getEnv("MINIO_BUCKET_RESULTS", "results"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
