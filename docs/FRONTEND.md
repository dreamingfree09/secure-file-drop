# Frontend / static UI

The application serves web/static/index.html at / and the asset directory at /static/. SFD_WEB_DIR overrides the default repo-relative web/static directory; the container works from /app.

The existing single-page interface provides registration, login, drag/drop upload with XMLHttpRequest progress, signed-link display and clipboard copy. Ordinary users see their own recent files. The environment administrator additionally sees global metrics, deletion and manual cleanup. Sign out revokes the server session and clears file/link/input state; delayed responses cannot render into a later session.

Registration checks the 8–72-byte bcrypt password bound in addition to matching passwords, username and email validation. Error text is escaped. The UI accurately describes private storage and SHA-256 integrity; it does not claim end-to-end encryption. Server-side authorization and limits remain authoritative.

Upload progress reports transferred bytes; it is not a hashing progress or capacity guarantee. The server has a configurable finite file limit, with a 50 GiB default and a smaller example configuration. Maximum-size throughput is not certified by this review.

Run Node UI-session unit tests with node --test tests/web/session.test.cjs. These isolated script tests are distinct from actual browser observations and real storage acceptance. Browser checks observed login/registration rendering, empty-field rejection and 73-byte rejection. Authenticated browser upload remains locally blocked by Docker Desktop startup; actual API/native/container evidence and its exact revision are in [the PR handover](https://github.com/dreamingfree09/secure-file-drop/pull/21) and [tracker](TRACKER.md).

Animated gradients, responsive styling and the Google Fonts Inter stylesheet are retained. This review does not introduce a frontend framework or build system.
