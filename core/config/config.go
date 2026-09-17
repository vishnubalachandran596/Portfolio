// Package config loads runtime configuration from environment variables.
package config

import (
	"fmt"
	"os"
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
	CheckoutCurrency  string
}

func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	siteName := os.Getenv("SITE_NAME")
	if siteName == "" {
		siteName = "Vishnu Balachandran"
	}

	checkoutCurrency := os.Getenv("CHECKOUT_CURRENCY")
	if checkoutCurrency == "" {
		checkoutCurrency = "USD"
	}

	return &Config{
		Port:          port,
		DatabaseURL:   dbURL,
		JWTSecret:     []byte(secret),
		SessionTTL:    7 * 24 * time.Hour,
		AdminEmail:    os.Getenv("ADMIN_BOOTSTRAP_EMAIL"),
		AdminPassword: os.Getenv("ADMIN_BOOTSTRAP_PASSWORD"),
		SiteName:      siteName,
		SiteURL:       os.Getenv("SITE_URL"),
		Domain:        os.Getenv("DOMAIN"),
		Environment:   env,

		RazorpayKeyID:     os.Getenv("RAZORPAY_KEY_ID"),
		RazorpayKeySecret: os.Getenv("RAZORPAY_KEY_SECRET"),
		CheckoutCurrency:  checkoutCurrency,
	}, nil
}

func (c *Config) IsProduction() bool {
	return c.Environment == "production"
}
