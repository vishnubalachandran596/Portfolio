package config

import (
	"strings"
	"testing"
)

func TestLoadStripsWindowsCarriageReturn(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://portfolio:portfolio@db:5432/portfolio?sslmode=disable\r")
	t.Setenv("JWT_SECRET", "secret\r")
	t.Setenv("LOCAL_CONTENT_DIR", "../portfolio-local-content\r")
	t.Setenv("PORT", "8080\r")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(cfg.DatabaseURL, "\r") || strings.Contains(string(cfg.JWTSecret), "\r") {
		t.Fatal("carriage return leaked into config")
	}
	if cfg.LocalContentDir != "../portfolio-local-content" {
		t.Fatalf("content dir %q", cfg.LocalContentDir)
	}
	if cfg.Port != "8080" {
		t.Fatalf("port %q", cfg.Port)
	}
}
