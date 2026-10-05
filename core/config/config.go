// Package config loads runtime configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Port          string
	DatabaseURL   string
	JWTSecret     []byte
	SessionTTL    time.Duration
	AdminEmail    string
	AdminPassword string
	SiteName      string
	SiteURL       string
	Domain        string
	Environment   string

	RazorpayKeyID     string
	RazorpayKeySecret string

	TelegramBotToken string
	TelegramChatID   string

	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string

	// Private Supabase Storage for paid lesson HTML. SecretKey is the
	// service-role (or sb_secret_) key and is enough on its own. Without
	// it, the anon key plus the dedicated storage-reader login is used.
	// That login is limited by storage policies to the course-content and articles buckets.
	SupabaseURL             string
	SupabaseSecretKey       string
	SupabaseAnonKey         string
	SupabaseStorageEmail    string
	SupabaseStoragePassword string

	// LocalContentDir, when set, is the directory of article HTML, course
	// HTML, and resume PDFs. Those files use the same object paths as
	// Supabase Storage. Production leaves this empty.
	LocalContentDir string

	// NvidiaAPIKey is a free-tier build.nvidia.com key (NIM) for the RAG
	// chatbot's embeddings and chat completion. The chatbot stays disabled
	// (hidden widget) until it is set -- see core/ai.Client.Enabled().
	NvidiaAPIKey string
}

func Load() (*Config, error) {
	dbURL := getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	secret := getenv("JWT_SECRET")
	if secret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	port := getenv("PORT")
	if port == "" {
		port = "8080"
	}

	env := getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	siteName := getenv("SITE_NAME")
	if siteName == "" {
		siteName = "Vishnu Balachandran"
	}

	return &Config{
		Port:          port,
		DatabaseURL:   dbURL,
		JWTSecret:     []byte(secret),
		SessionTTL:    7 * 24 * time.Hour,
		AdminEmail:    getenv("ADMIN_BOOTSTRAP_EMAIL"),
		AdminPassword: getenv("ADMIN_BOOTSTRAP_PASSWORD"),
		SiteName:      siteName,
		SiteURL:       getenv("SITE_URL"),
		Domain:        getenv("DOMAIN"),
		Environment:   env,

		RazorpayKeyID:     getenv("RAZORPAY_KEY_ID"),
		RazorpayKeySecret: getenv("RAZORPAY_KEY_SECRET"),

		TelegramBotToken: getenv("TELEGRAM_BOT_TOKEN"),
		TelegramChatID:   getenv("TELEGRAM_CHAT_ID"),

		GoogleClientID:     getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: getenv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURL:  getenv("GOOGLE_REDIRECT_URL"),

		SupabaseURL:             getenv("SUPABASE_URL"),
		SupabaseSecretKey:       getenv("SUPABASE_SECRET_KEY"),
		SupabaseAnonKey:         getenv("SUPABASE_ANON_KEY"),
		SupabaseStorageEmail:    getenv("SUPABASE_STORAGE_EMAIL"),
		SupabaseStoragePassword: getenv("SUPABASE_STORAGE_PASSWORD"),

		LocalContentDir: getenv("LOCAL_CONTENT_DIR"),

		NvidiaAPIKey: getenv("NVIDIA_API_KEY"),
	}, nil
}

// getenv trims spaces and a trailing carriage return. A .env saved by
// Windows Notepad is CRLF; Compose on Docker Desktop usually strips that,
// and this keeps a host `go run` on Windows from seeing the CR as part of
// the value.
func getenv(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func (c *Config) IsProduction() bool {
	return c.Environment == "production"
}
