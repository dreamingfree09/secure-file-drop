// Package server implements the HTTP server and HTTP handlers for
// Secure File Drop. It wires together the HTTP routes, dependencies
// (database, MinIO client), and provides lifecycle helpers used by
// tests and the production binary.
package server
