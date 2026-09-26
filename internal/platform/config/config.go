package config

import "os"

// Config is shared across all three binaries (identity-api, video-api,
// video-worker) for simplicity; each one only reads the fields it needs,
// which is an accepted trade-off over three parallel config packages —
// unused env vars are harmless (see docs/adr/0007).
type Config struct {
	Port                string
	IdentityPort        string
	WorkerMetricsPort   string
	IdentityDatabaseURL string
	VideoDatabaseURL    string
	JWTSecret           string
	RabbitMQURL         string
	MinIOEndpoint       string
	MinIOPublicEndpoint string
	MinIOAccessKey      string
	MinIOSecretKey      string
	MinIOVideosBucket   string
	MinIOResultsBucket  string
	SMTPHost            string
	SMTPPort            string
	SMTPUser            string
	SMTPPassword        string
	SMTPFrom            string
	FrontendOrigin      string
}

func Load() Config {
	return Config{
		Port:                getEnv("PORT", "8080"),
		IdentityPort:        getEnv("IDENTITY_PORT", "8081"),
		WorkerMetricsPort:   getEnv("WORKER_METRICS_PORT", "9102"),
		IdentityDatabaseURL: os.Getenv("IDENTITY_DATABASE_URL"),
		VideoDatabaseURL:    os.Getenv("VIDEO_DATABASE_URL"),
		JWTSecret:           os.Getenv("JWT_SECRET"),
		RabbitMQURL:         os.Getenv("RABBITMQ_URL"),
		MinIOEndpoint:       os.Getenv("MINIO_ENDPOINT"),
		MinIOPublicEndpoint: getEnv("MINIO_PUBLIC_ENDPOINT", os.Getenv("MINIO_ENDPOINT")),
		MinIOAccessKey:      os.Getenv("MINIO_ACCESS_KEY"),
		MinIOSecretKey:      os.Getenv("MINIO_SECRET_KEY"),
		MinIOVideosBucket:   getEnv("MINIO_BUCKET_VIDEOS", "videos"),
		MinIOResultsBucket:  getEnv("MINIO_BUCKET_RESULTS", "results"),
		SMTPHost:            os.Getenv("SMTP_HOST"),
		SMTPPort:            getEnv("SMTP_PORT", "2525"),
		SMTPUser:            os.Getenv("SMTP_USER"),
		SMTPPassword:        os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:            getEnv("SMTP_FROM", "no-reply@fiapx.local"),
		FrontendOrigin:      getEnv("FRONTEND_ORIGIN", "http://localhost:3000"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
