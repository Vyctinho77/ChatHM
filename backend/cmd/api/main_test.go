package main

import (
	"errors"
	"net"
	"testing"
)

func TestOccupiedPortFailsBeforeDatabaseAndWhatsApp(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	t.Setenv("HTTP_ADDR", listener.Addr().String())
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "invalid-dsn")
	t.Setenv("ENCRYPTION_KEY", "unused-before-binding")
	var op *net.OpError
	if err := run(); !errors.As(err, &op) || op.Op != "listen" {
		t.Fatalf("expected listen failure before database setup, got %v", err)
	}
}
