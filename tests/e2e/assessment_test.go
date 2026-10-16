package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

// Uses only the parent test's fresh, verified disposable database and bucket.
func assessBoundaries(ctx context.Context, t *testing.T, conn *sql.DB, mc *minio.Client, bucket, base string,
	admin, alice *http.Cookie,
	request func(string, string, io.Reader, *http.Cookie, map[string]string) (int, []byte, []*http.Cookie),
	create func(string, int, *http.Cookie) string,
	upload func(string, []byte, *http.Cookie) int) {
	t.Helper()
	expect := func(code, want int) {
		t.Helper()
		if code != want {
			t.Fatalf("HTTP %d want %d", code, want)
		}
	}
	body := func(s string) io.Reader { return strings.NewReader(s) }
	t.Run("assessment-route-matrix", func(t *testing.T) {
		for _, tc := range []struct{ method, path string }{
			{"GET", "/me"}, {"GET", "/files"}, {"POST", "/files"}, {"POST", "/upload?id=" + uuid.NewString()},
			{"POST", "/links"}, {"POST", "/logout"}, {"GET", "/admin/files"}, {"DELETE", "/admin/files/" + uuid.NewString()},
			{"POST", "/admin/cleanup"}, {"GET", "/metrics"},
		} {
			code, _, _ := request(tc.method, base+tc.path, body("{}"), nil, nil)
			expect(code, 401)
		}
		for _, path := range []string{"/health", "/ready", "/version", "/"} {
			code, _, _ := request("GET", base+path, nil, nil, nil)
			expect(code, 200)
		}
		for _, tc := range []struct{ method, path string }{{"GET", "/admin/files"}, {"GET", "/metrics"}, {"POST", "/admin/cleanup"}, {"DELETE", "/admin/files/" + uuid.NewString()}} {
			code, _, _ := request(tc.method, base+tc.path, nil, alice, nil)
			expect(code, 403)
		}
		for _, id := range []string{"bad-id", uuid.NewString()} {
			want := 400
			if id != "bad-id" {
				want = 404
			}
			code, _, _ := request("POST", base+"/links", body(fmt.Sprintf("{\"id\":%q}", id)), alice, nil)
			expect(code, want)
			expect(upload(id, []byte("x"), alice), want)
			code, _, _ = request("DELETE", base+"/admin/files/"+id, nil, admin, nil)
			expect(code, want)
		}
		t.Log("PASS: actual anonymous/ordinary/admin route matrix; malformed/nonexistent IDs")
	})
	t.Run("assessment-ownerless", func(t *testing.T) {
		id := create("historical-ownerless.txt", 1, alice)
		if _, err := conn.ExecContext(ctx, "UPDATE files SET user_id=NULL WHERE id=$1", id); err != nil {
			t.Fatal(err)
		}
		expect(upload(id, []byte("x"), alice), 404)
		code, _, _ := request("POST", base+"/links", body(fmt.Sprintf("{\"id\":%q}", id)), alice, nil)
		expect(code, 404)
		code, b, _ := request("GET", base+"/files", nil, alice, nil)
		expect(code, 200)
		if bytes.Contains(b, []byte(id)) {
			t.Fatal("ownerless row leaked")
		}
		expect(upload(id, []byte("x"), admin), 200)
		code, _, _ = request("POST", base+"/links", body(fmt.Sprintf("{\"id\":%q}", id)), admin, nil)
		expect(code, 200)
		code, _, _ = request("DELETE", base+"/admin/files/"+id, nil, admin, nil)
		expect(code, 204)
		t.Log("PASS: historical ownerless rows administrator-only")
	})
	t.Run("assessment-upload-bounds", func(t *testing.T) {
		for _, size := range []int{0, 1, 1024} {
			id := create("bounded.bin", 0, alice)
			expect(upload(id, bytes.Repeat([]byte("x"), size), alice), 200)
			var actual, hashed int
			if err := conn.QueryRowContext(ctx, "SELECT size_bytes,sha256_bytes FROM files WHERE id=$1", id).Scan(&actual, &hashed); err != nil || actual != size || hashed != size {
				t.Fatal("actual-size mismatch")
			}
		}
		id := create("truncated.bin", 1, alice)
		data := "--fixture\r\nContent-Disposition: form-data; name=\"file\"; filename=\"x\"\r\n\r\nabc"
		code, _, _ := request("POST", base+"/upload?id="+id, body(data), alice, map[string]string{"Content-Type": "multipart/form-data; boundary=fixture"})
		if code < 400 {
			t.Fatalf("truncated upload accepted: %d", code)
		}
		var status string
		if err := conn.QueryRowContext(ctx, "SELECT status FROM files WHERE id=$1", id).Scan(&status); err != nil || status != "failed" {
			t.Fatalf("truncated state %q, %v", status, err)
		}

		for _, malformed := range []string{
			"--fixture\r\nContent-Disposition: form-data; name=\"field\"\r\n\r\nx\r\n--fixture--\r\n",
			"--fixture\r\nContent-Disposition: form-data; name=\"file\"; filename=\"x\"\r\n\r\nx\r\n--fixture\r\nContent-Disposition: form-data; name=\"file\"; filename=\"y\"\r\n\r\ny\r\n--fixture--\r\n",
			"--fixture\r\nContent-Disposition: form-data; name=\"file\"; filename=\"x\"\r\n\r\nx\r\n--fixture-bad",
		} {
			badID := create("multipart-policy.bin", 1, alice)
			code, _, _ := request("POST", base+"/upload?id="+badID, body(malformed), alice, map[string]string{"Content-Type": "multipart/form-data; boundary=fixture"})
			if code != 400 {
				t.Fatalf("multipart policy HTTP %d", code)
			}
			var key, status string
			if err := conn.QueryRowContext(ctx, "SELECT object_key,status FROM files WHERE id=$1", badID).Scan(&key, &status); err != nil || status != "failed" {
				t.Fatal("malformed lifecycle", err, status)
			}
			if _, err := mc.StatObject(ctx, bucket, key, minio.StatObjectOptions{}); err == nil {
				t.Fatal("malformed object retained")
			}
		}
		t.Log("PASS: empty/exact-limit/declared-actual size and truncated-body lifecycle")
	})
	t.Run("assessment-commit-rejection", func(t *testing.T) {
		id := create("commit-rejected.bin", 1, alice)

		// A fixed trigger is safe here: the database is newly owned by this test,
		// and this subtest has no concurrent upload. No input is interpolated in SQL.
		const ddl = "CREATE FUNCTION assessment_commit_rejection() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.status='hashed' THEN RAISE EXCEPTION 'synthetic commit rejection'; END IF; RETURN NEW; END $$; CREATE CONSTRAINT TRIGGER assessment_commit_rejection AFTER UPDATE ON files DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION assessment_commit_rejection()"
		if _, err := conn.ExecContext(ctx, ddl); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if _, err := conn.ExecContext(ctx, "DROP TRIGGER assessment_commit_rejection ON files; DROP FUNCTION assessment_commit_rejection()"); err != nil {
				t.Error(err)
			}
		}()

		expect(upload(id, []byte("x"), alice), 500)
		var key, status string
		if err := conn.QueryRowContext(ctx, "SELECT object_key,status FROM files WHERE id=$1", id).Scan(&key, &status); err != nil || status != "failed" {
			t.Fatalf("rejected commit state %q, %v", status, err)
		}
		if _, err := mc.StatObject(ctx, bucket, key, minio.StatObjectOptions{}); err == nil {
			t.Fatal("rejected commit object retained")
		}
		t.Log("PASS: known-rejected commit compensation; separate fault fixtures cover ambiguous acknowledgement")
	})
	t.Run("assessment-session-and-request-bounds", func(t *testing.T) {
		cookies := []*http.Cookie{}
		for i := 0; i < 2; i++ {
			code, _, cs := request("POST", base+"/login", body("{\"username\":\"alice\",\"password\":\" SpacePass123 \"}"), nil, nil)
			expect(code, 200)
			if len(cs) != 1 {
				t.Fatal("missing cookie")
			}
			cookies = append(cookies, cs[0])
		}
		if cookies[0].Value == cookies[1].Value {
			t.Fatal("sessions identical")
		}
		code, _, _ := request("POST", base+"/logout", nil, cookies[0], nil)
		expect(code, 204)
		code, _, _ = request("GET", base+"/me", nil, cookies[0], nil)
		expect(code, 401)
		code, _, _ = request("GET", base+"/me", nil, cookies[1], nil)
		expect(code, 200)
		for _, route := range []string{"/login", "/register", "/files", "/links"} {
			c := alice
			if route == "/login" || route == "/register" {
				c = nil
			}
			code, _, _ = request("POST", base+route, body("{\"padding\":\""+strings.Repeat("x", 17000)+"\"}"), c, nil)
			if code < 400 {
				t.Fatalf("oversized JSON accepted: %s", route)
			}
		}

		for _, route := range []string{"/login", "/register", "/files", "/links"} {
			c := alice
			if route == "/login" || route == "/register" {
				c = nil
			}
			for _, suffix := range []string{"{}", "!", strings.Repeat(" ", 17<<10)} {
				code, _, _ := request("POST", base+route, body("{}"+suffix), c, nil)
				want := 400
				if len(suffix) > 16<<10 {
					want = 413
				}
				expect(code, want)
			}
		}
		for _, origin := range []string{"null", "https://foreign.example.test"} {
			code, _, _ = request("POST", base+"/files", body("{}"), alice, map[string]string{"Origin": origin})
			expect(code, 403)
		}
		for _, tok := range []string{"x", "..", "%%%..", "e30.invalid", "a.b.c"} {
			code, _, _ = request("GET", base+"/download?token="+url.QueryEscape(tok), nil, nil, nil)
			expect(code, 401)
		}
		t.Log("PASS: independent sessions/revocation; JSON bounds; Origin and malformed token rejection")
	})
}
