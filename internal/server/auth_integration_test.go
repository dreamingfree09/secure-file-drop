package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

// This suite only accepts a fresh local database with an explicit fixture marker.
// The harness creates and removes its own labeled PostgreSQL container/volume.
func TestAuthenticatedIdentities(t *testing.T) {
	dsn := os.Getenv("SFD_AUTH_TEST_DSN")
	if dsn == "" {
		t.Skip("requires the isolated auth fixture; this skip is not acceptance")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "127.0.0.1" || !strings.HasPrefix(u.Path, "/sfd_auth_") || os.Getenv("SFD_AUTH_TEST_OWNER") == "" {
		t.Fatal("refusing unowned database target")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal("open fixture database")
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error("close fixture database")
		}
	}()
	var marker string
	if err := db.QueryRow("SELECT owner FROM sfd_test_owner").Scan(&marker); err != nil || marker != os.Getenv("SFD_AUTH_TEST_OWNER") {
		t.Fatal("fixture ownership mismatch")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("Synthetic7-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	a := AuthConfig{DB: db, AdminUser: "admin", AdminPass: string(hash), SessionSecret: "synthetic-session-signing-secret-only", SessionTTL: time.Hour}
	addUser := func(name string, admin, active, verified bool) string {
		t.Helper()
		id := uuid.NewString()
		if _, err := db.Exec("INSERT INTO users (id,email,username,password_hash,is_admin,is_active,email_verified) VALUES ($1,$2,$3,$4,$5,$6,$7)", id, id+"@example.invalid", name, string(hash), admin, active, verified); err != nil {
			t.Fatal("insert synthetic user")
		}
		return id
	}
	ordinary := addUser("ordinary-"+uuid.NewString(), false, true, true)
	// A registered username matching the environment name must never confer admin.
	collision := addUser("admin", false, true, true)
	roleAdmin := addUser("role-"+uuid.NewString(), true, true, true)
	inactive := addUser("inactive-"+uuid.NewString(), false, false, true)
	unverified := addUser("unverified-"+uuid.NewString(), false, true, false)
	request := func(handler http.Handler, method string, cookie *http.Cookie, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "https://localhost/", bytes.NewBufferString(body))
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	expect := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("HTTP status: got %d want %d", w.Code, status)
		}
	}
	login := func(username, password string) *http.Cookie {
		t.Helper()
		b, _ := json.Marshal(map[string]string{"username": username, "password": password})
		w := request(a.loginHandler(), "POST", nil, string(b))
		expect(w, 200)
		cs := w.Result().Cookies()
		if len(cs) != 1 || !cs[0].Secure || !cs[0].HttpOnly || cs[0].SameSite != http.SameSiteLaxMode {
			t.Fatal("session cookie policy")
		}
		return cs[0]
	}
	lookup := func(id string) string {
		t.Helper()
		var n string
		if db.QueryRow("SELECT username FROM users WHERE id=$1", id).Scan(&n) != nil {
			t.Fatal("fixture user lookup")
		}
		return n
	}
	grant := func(sub string, admin bool, ttl time.Duration) *http.Cookie {
		t.Helper()
		cfg := a
		cfg.SessionTTL = ttl
		tok, exp, err := cfg.makeSessionToken(sub, admin)
		if err != nil {
			t.Fatal(err)
		}
		if ttl < 0 {
			parts := strings.Split(tok, ".")
			payload, _ := decodeSession(parts[0])
			payload.Exp = time.Now().Add(ttl).Unix()
			encoded, _ := encodeSession(payload)
			tok = encoded + "." + signPayload(a.secretBytes(), encoded)
			exp = time.Now().Add(ttl)
		}
		if _, err = db.Exec("INSERT INTO auth_sessions(token_hash,expires_at) VALUES($1,$2)", sessionHash(tok), exp); err != nil {
			t.Fatal("fixture session insert")
		}
		return &http.Cookie{Name: a.cookieName(), Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, Value: tok}
	}
	adminRoute := a.requireAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	t.Run("environment administrator identity and permission", func(t *testing.T) {
		// Database-first login preserves the established credential precedence.
		// A distinct environment password proves that the fallback issued the marker.
		envHash, _ := bcrypt.GenerateFromPassword([]byte("Environment8-only"), bcrypt.MinCost)
		a.AdminPass = string(envHash)
		c := login("admin", "Environment8-only")
		w := request(a.meHandler(), "GET", c, "")
		expect(w, 200)
		var me struct {
			Username string `json:"username"`
			Admin    bool   `json:"is_admin"`
		}
		if json.Unmarshal(w.Body.Bytes(), &me) != nil || me.Username != "admin" || !me.Admin {
			t.Fatal("environment admin identity")
		}
		expect(request(adminRoute, "GET", c, ""), 204)
		r := httptest.NewRequest("GET", "https://localhost/", nil)
		r.AddCookie(c)
		if _, err := a.getCurrentUser(r); err != errRegisteredAccountRequired {
			t.Fatal("environment identity cannot become a database owner")
		}
	})
	t.Run("registered identity collision and roles", func(t *testing.T) {
		for _, tc := range []struct {
			id      string
			isAdmin bool
		}{{ordinary, false}, {collision, false}, {roleAdmin, true}} {
			c := login(lookup(tc.id), "Synthetic7-password")
			w := request(a.meHandler(), "GET", c, "")
			expect(w, 200)
			var me struct {
				Username string `json:"username"`
				Admin    bool   `json:"is_admin"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &me)
			if me.Username != lookup(tc.id) || me.Admin != tc.isAdmin {
				t.Fatal("registered identity or role mismatch")
			}
			want := 403
			if tc.isAdmin {
				want = 204
			}
			expect(request(adminRoute, "GET", c, ""), want)
		}
	})
	t.Run("invalid unknown expired tampered and legacy sessions", func(_ *testing.T) {
		valid := grant(ordinary, false, time.Hour)
		cases := make([]*http.Cookie, 0, 10)
		cases = append(cases, nil, &http.Cookie{Name: a.cookieName(), Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, Value: "invalid"}, &http.Cookie{Name: a.cookieName(), Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, Value: valid.Value + "x"}, grant(uuid.NewString(), false, time.Hour), grant("not-a-uuid", false, time.Hour), grant(inactive, false, time.Hour), grant(unverified, false, time.Hour), grant(ordinary, false, -time.Hour))
		// Authenticated legacy payloads are intentionally invalidated by the migration.
		encoded, _ := encodeSession(sessionPayload{Sub: "admin", Exp: time.Now().Add(time.Hour).Unix()})
		cases = append(cases, &http.Cookie{Name: a.cookieName(), Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, Value: encoded + "." + signPayload(a.secretBytes(), encoded)})
		// A marker cannot promote a different subject even if that subject is a user.
		cases = append(cases, grant(ordinary, true, time.Hour))
		for _, c := range cases {
			expect(request(a.meHandler(), "GET", c, ""), 401)
		}
		expect(request(a.loginHandler(), "POST", nil, `{"username":"unknown","password":"Synthetic7-password"}`), 401)
		expect(request(a.loginHandler(), "POST", nil, `{"username":"admin","password":"wrong"}`), 401)
	})
	t.Run("registered ownership across file and resumable routes", func(t *testing.T) {
		mine := login(lookup(ordinary), "Synthetic7-password")
		other := login(lookup(collision), "Synthetic7-password")
		fileID := uuid.NewString()
		if _, err := db.Exec("INSERT INTO files(id,object_key,orig_name,content_type,size_bytes,user_id) VALUES($1,$2,'fixture.txt','text/plain',1,$3)", fileID, "test/"+fileID, ordinary); err != nil {
			t.Fatal("synthetic ownership record")
		}
		cfg := Config{DB: db, Auth: a}
		srv := Server{db: db, authCfg: a}
		// Authentication and ownership fail before storage is accessed (nil fixture client).
		upload := cfg.uploadHandler(db, nil, "fixture")
		req := httptest.NewRequest("POST", "https://localhost/upload?id="+fileID, nil)
		req.AddCookie(other)
		w := httptest.NewRecorder()
		upload.ServeHTTP(w, req)
		expect(w, 404)
		expect(request(cfg.createLinkHandler(db), "POST", other, `{"id":"`+fileID+`"}`), 404)
		expect(request(cfg.createLinkHandler(db), "POST", nil, `{"id":"`+fileID+`"}`), 401)
		create := a.requireAuth(srv.tusCreateHandler())
		tusReq := func(c *http.Cookie) *httptest.ResponseRecorder {
			r := httptest.NewRequest("POST", "https://localhost/upload/resumable", nil)
			r.Header.Set("Upload-Length", "1")
			r.Header.Set("Upload-Metadata", "file_id "+fileID)
			if c != nil {
				r.AddCookie(c)
			}
			w := httptest.NewRecorder()
			create.ServeHTTP(w, r)
			return w
		}
		expect(tusReq(nil), 401)
		expect(tusReq(other), 404)
		own := tusReq(mine)
		expect(own, 201)
		location := own.Header().Get("Location")
		for _, method := range []string{"HEAD", "PATCH"} {
			var handler http.Handler = srv.tusHeadHandler()
			if method == "PATCH" {
				handler = srv.tusPatchHandler()
			}
			handler = a.requireAuth(handler)
			for _, tc := range []struct {
				cookie *http.Cookie
				status int
			}{{nil, 401}, {other, 404}} {
				r := httptest.NewRequest(method, "https://localhost"+location, nil)
				if tc.cookie != nil {
					r.AddCookie(tc.cookie)
				}
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				expect(w, tc.status)
			}
		}
		r := httptest.NewRequest("HEAD", "https://localhost"+location, nil)
		r.AddCookie(mine)
		w = httptest.NewRecorder()
		a.requireAuth(srv.tusHeadHandler()).ServeHTTP(w, r)
		expect(w, 200)
	})
	t.Run("database expiry and configured-name UUID collision", func(t *testing.T) {
		c := grant(ordinary, false, time.Hour)
		originalName := a.AdminUser
		a.AdminUser = ordinary
		expect(request(a.requireAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })), "GET", c, ""), 403)
		a.AdminUser = originalName
		if _, err := db.Exec("UPDATE auth_sessions SET expires_at=CURRENT_TIMESTAMP-INTERVAL '1 second' WHERE token_hash=$1", sessionHash(c.Value)); err != nil {
			t.Fatal("expire synthetic session")
		}
		expect(request(a.meHandler(), "GET", c, ""), 401)
	})

	t.Run("session isolation and persistent logout", func(t *testing.T) {
		one := login(lookup(ordinary), "Synthetic7-password")
		two := login(lookup(ordinary), "Synthetic7-password")
		other := login("admin", "Environment8-only")
		if one.Value == two.Value {
			t.Fatal("sessions must be unique")
		}
		expect(request(a.logoutHandler(), "POST", one, ""), 200)
		// A new AuthConfig has no process-local revocation state.
		restarted := a
		expect(request(restarted.meHandler(), "GET", one, ""), 401)
		expect(request(restarted.meHandler(), "GET", two, ""), 200)
		expect(request(restarted.meHandler(), "GET", other, ""), 200)
		expect(request(a.logoutHandler(), "POST", one, ""), 401)
		var stored string
		if db.QueryRow("SELECT token_hash FROM auth_sessions WHERE token_hash=$1", sessionHash(two.Value)).Scan(&stored) != nil || stored == two.Value || len(stored) != 64 {
			t.Fatal("only token hash is persisted")
		}
		if _, err := db.Exec("UPDATE users SET is_active=false WHERE id=$1", ordinary); err != nil {
			t.Fatal("deactivate synthetic account")
		}
		expect(request(a.meHandler(), "GET", two, ""), 401)
	})
	t.Run("HTTP methods and safe errors", func(t *testing.T) {
		expect(request(a.loginHandler(), "GET", nil, ""), 405)
		expect(request(a.loginHandler(), "POST", nil, "{"), 400)
		expect(request(a.logoutHandler(), "GET", nil, ""), 405)
		c := login("admin", "Environment8-only")
		expect(request(a.meHandler(), "POST", c, ""), 405)
		broken := a
		broken.DB = nil
		w := request(broken.meHandler(), "GET", c, "")
		expect(w, 500)
		if strings.Contains(w.Body.String(), a.SessionSecret) || strings.Contains(w.Body.String(), a.AdminPass) || strings.Contains(w.Body.String(), c.Value) {
			t.Fatal("secret disclosed")
		}
	})
	t.Run("bounded expired session reclamation preserves active sessions", func(t *testing.T) {
		active := login("admin", "Environment8-only")
		if _, err := db.Exec(`INSERT INTO auth_sessions(token_hash,expires_at) SELECT md5(i::text)||md5('fixture-expired-'||i::text),CURRENT_TIMESTAMP-INTERVAL '1 hour' FROM generate_series(1,300) i`); err != nil {
			t.Fatal("seed expired synthetic sessions")
		}
		count := func() int {
			t.Helper()
			var n int
			if db.QueryRow("SELECT count(*) FROM auth_sessions WHERE expires_at <= CURRENT_TIMESTAMP").Scan(&n) != nil {
				t.Fatal("expired session count")
			}
			return n
		}
		before := count()
		_ = login("admin", "Environment8-only")
		if count() != before-256 {
			t.Fatal("cleanup must reclaim only one bounded batch")
		}
		expect(request(a.meHandler(), "GET", active, ""), 200)
		_ = login("admin", "Environment8-only")
		if count() != 0 {
			t.Fatal("successive logins reclaim remaining expired sessions")
		}
		expect(request(a.meHandler(), "GET", active, ""), 200)
	})

}
