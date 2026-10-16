package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"strconv"
	"testing"

	"github.com/minio/minio-go/v7"
)

// Fixture recovery exercises pg_dump/psql and one synthetic object only; it
// does not certify application disaster recovery or touch any real backup.
func assessFixtureRecovery(ctx context.Context, t *testing.T, conn *sql.DB, mc *minio.Client,
	database, bucket, key, pg, run string, docker func(...string) string) {
	t.Helper()
	var current string
	if err := conn.QueryRowContext(ctx, "SELECT current_database()").Scan(&current); err != nil || current != database {
		t.Fatal("recovery source target mismatch")
	}
	var want int
	if err := conn.QueryRowContext(ctx, "SELECT count(*) FROM files").Scan(&want); err != nil {
		t.Fatal(err)
	}
	// Parent has verified this container's assignment label and fresh volume.
	if got := docker("container", "inspect", "--format", "{{index .Config.Labels \"sfd.quality-run\"}}", pg); got != run {
		t.Fatal("recovery container ownership mismatch")
	}
	dump := "/tmp/" + run + "-fixture.sql"
	docker("exec", pg, "pg_dump", "-U", "sfd", "-d", database, "--no-owner", "-f", dump)
	recovered := database + "_recovery"
	if _, err := conn.ExecContext(ctx, "CREATE DATABASE "+recovered); err != nil {
		t.Fatal(err)
	}
	docker("exec", pg, "psql", "-U", "sfd", "-d", recovered, "-v", "ON_ERROR_STOP=1", "-f", dump)
	count := docker("exec", pg, "psql", "-U", "sfd", "-d", recovered, "-Atc", "SELECT count(*) FROM files")
	if count != strconv.Itoa(want) {
		t.Fatal("recovered file row count mismatch")
	}
	gotVersion := docker("exec", pg, "psql", "-U", "sfd", "-d", recovered, "-Atc", "SELECT version::text||':'||dirty::text FROM schema_migrations")
	if gotVersion != "4:false" {
		t.Fatalf("recovered migration state %s", gotVersion)
	}
	obj, err := mc.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(io.LimitReader(obj, 2048))
	_ = obj.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || len(data) > 1024 {
		t.Fatal("unexpected recovery fixture size")
	}
	dest := run + "-recovered"
	if err := mc.MakeBucket(ctx, dest, minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := mc.PutObject(ctx, dest, "synthetic-restored", bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{}); err != nil {
		t.Fatal(err)
	}
	restored, err := mc.GetObject(ctx, dest, "synthetic-restored", minio.GetObjectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(io.LimitReader(restored, 2048))
	_ = restored.Close()
	if err != nil || !bytes.Equal(actual, data) {
		t.Fatal("restored fixture bytes differ")
	}
	t.Log("PASS: disposable pg_dump/psql restore, migration-state/row-count checks and bounded object-copy readback; service-wide recovery unverified")
}
