# Architecture

The backend authenticates environment administrator credentials or database users. Sessions contain HMAC-signed subject, expiry, a random identifier and an explicit administrator marker. A session is also required to have an unexpired token-hash record in PostgreSQL. Logout removes that record; ordinary-user requests recheck active-account status. Roles come from authenticated credential sources, not usernames.

PostgreSQL stores users, sessions and file metadata. Ordinary files have user_id ownership; administrator-created and historical ownerless records remain accessible to the administrator. Row locks serialize uploads with deletion; cleanup skips locked rows. Upload updates pending → stored → hashed in a transaction. Processing failures remove stored objects and retain a failed record for cleanup. The externally visible successful upload state remains hashed; ready is also downloadable.

The backend writes generated object keys to private MinIO storage. Startup requires an existing bucket without anonymous bucket-policy grants. It does not silently alter storage policy. Object removal precedes database deletion so a storage failure retains retryable metadata. Completed objects are streamed to a temporary local file for the real C/OpenSSL SHA-256 utility; both temporary and object byte counts must match. Hashing uses bounded memory but requires temporary disk space up to the configured upload limit.

Signed download links are bearer authorization. HMAC verification and expiry are independent of session authentication. Metadata ownership is checked when generating a link; anyone holding it may download while valid. Deletion invalidates the file reference. Downloads use object-stat length and a safely formatted attachment filename.

Startup automatically applies embedded forward migrations before serving. Shutdown cancels and waits for cleanup. Assets default to web/static relative to the working directory, with SFD_WEB_DIR override. The container runs from /app as a non-root user and includes libcrypto3, CA certificates and the C executable.

Production TLS termination, host routing and ingress rate limiting remain deployment responsibilities. Browser state-changing requests with an Origin must match the configured public origin; cookies are HttpOnly, SameSite=Lax and Secure for HTTPS configuration. Forwarded request headers are not trusted as a canonical public origin. See [usage](USAGE.md) and [current evidence](TRACKER.md).
