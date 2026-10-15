// Command migrate applies embedded forward migrations to an explicitly configured database.
package main

import (
	"log"
	"os"
	"secure-file-drop/internal/db"
	"secure-file-drop/internal/server"
)

func main() {
	conn, err := server.OpenDB(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal("cannot connect to the explicitly configured database")
	}
	defer func() { _ = conn.Close() }()
	if err = db.RunMigrations(conn); err != nil {
		log.Fatalf("migration failed: %v", err)
	}
	log.Print("migrations complete")
}
