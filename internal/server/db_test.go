package server

import "testing"

func TestOpenDB_Empty(t *testing.T) {
	if _, err := OpenDB(""); err == nil {
		t.Fatal("expected error for empty DATABASE_URL")
	}
}

func TestOpenDB_BadDSN(t *testing.T) {
	// Non-empty but no DB running -- should return an error (no panic)
	if _, err := OpenDB("postgres://invalid:invalid@localhost:9999/bad?sslmode=disable"); err == nil {
		t.Fatal("expected error for bad DSN")
	}
}
