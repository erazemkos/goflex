# Step 11 — Auth and Sessions

## Goal

Provide a secure, opinionated authentication primitive that covers the common
SaaS case: email/password auth, cookie-based sessions, CSRF protection,
middleware to guard routes, and a frontend `UseUser` hook.

## Deliverables

1. `pkg/auth/auth.go`:
   - `NewAuth(cfg Config) *Auth`
   - `Auth.Middleware()` — attaches session user to `gin.Context`.
   - `Auth.RequireUser()` — middleware returning 401 if unauthenticated.
   - `Auth.Login(c *gin.Context, userID string)` — issues session cookie.
   - `Auth.Logout(c *gin.Context)` — clears cookie.
2. Session storage:
   - `sessionstore.Memory` (dev).
   - `sessionstore.Cookie` (signed, stateless).
   - `sessionstore.GORM` (DB-backed).
   - Interface: `Get`, `Set`, `Delete`, `Touch`.
3. CSRF protection via double-submit cookie + `X-CSRF-Token` header.
4. Password utilities: `auth.HashPassword`, `auth.ComparePassword` (argon2id).
5. Rate-limited login endpoint scaffolding in `pkg/auth/handlers.go`.
6. Frontend:
   - `hooks.UseUser() *User` — returns current user or nil.
   - `auth.RequireLogin(children...)` — redirects to `/login` if not
     authenticated.

## Implementation notes

### Config

```go
type Config struct {
    SecretKey       []byte
    CookieName      string
    CookieDomain    string
    CookieSecure    bool
    CookieSameSite  http.SameSite
    SessionDuration time.Duration
    Store           sessionstore.Store
    UserLoader      func(ctx context.Context, id string) (User, error)
}
```

### Cookie properties (defaults)

- `HttpOnly: true`
- `Secure: true` in production, `false` in dev (override-able)
- `SameSite: Lax`
- Signed with HMAC using `SecretKey`.

### CSRF

- On first request, server sets a `csrf_token` cookie (readable by JS).
- The frontend HTTP client reads the cookie and adds
  `X-CSRF-Token: <value>` to every mutating request (POST/PUT/PATCH/DELETE).
- The server middleware compares header vs cookie; mismatch → 403.
- GET/HEAD/OPTIONS are exempt.

### User loader

Decouples the auth package from any particular user model. Apps supply a
`UserLoader` that looks up the user by id (from DB, cache, etc.). The
loaded user is stored on `gin.Context` via a key and exposed via
`auth.CurrentUser(c)`.

### Frontend user bootstrap

On first page load, the server may inline `window.__USER__` from the session.
The `UseUser` hook reads that as the initial value; subsequent updates come
from query-layer calls to `/api/auth/me`.

## Testing scenarios

### T11.1 — Password hashing

- `HashPassword("secret")` returns a string with a non-empty argon2 prefix.
- `ComparePassword(hash, "secret")` returns true.
- `ComparePassword(hash, "wrong")` returns false.
- Different invocations produce different hashes (due to salt).

### T11.2 — Login issues a session cookie

- `POST /api/auth/login` with correct credentials returns 200 and a
  `Set-Cookie` header named per `Config.CookieName`.
- The cookie is HttpOnly and has the configured SameSite.

### T11.3 — Middleware attaches user

- A request with a valid session cookie hits a handler where
  `auth.CurrentUser(c) != nil` and has the expected ID.

### T11.4 — RequireUser guards routes

- Without session: `GET /api/me` returns 401.
- With session: returns 200.

### T11.5 — Logout clears cookie

- `POST /api/auth/logout` returns `Set-Cookie` with MaxAge<=0 for the
  session cookie.
- Follow-up request is unauthenticated.

### T11.6 — Session expiration

- With `SessionDuration=1h`, a session older than 1h returns 401.
- Active use before expiration `Touch`es the expiry (sliding expiration).

### T11.7 — CSRF enforcement

- `POST /api/todos` without `X-CSRF-Token` returns 403 with code
  `csrf_failed`.
- With matching `X-CSRF-Token` and cookie: 200.
- `GET /api/todos` requires no CSRF token.

### T11.8 — Cookie session store

- The `Cookie` store encodes session data in a signed cookie and decodes it
  correctly on the next request.
- Tampering (flip a byte) causes the decoder to reject the cookie and
  return an unauthenticated request.

### T11.9 — GORM session store

- Creating, reading, updating, and deleting a session row roundtrips
  correctly with a test SQLite DB.
- Expired rows are cleaned up by a `Cleanup()` method.

### T11.10 — Rate limiting on login

- 10 failed attempts within 60 seconds from the same IP return 429.
- After the window, attempts succeed again.

### T11.11 — Frontend UseUser

- With an inlined `window.__USER__`, first render returns the user.
- After `Logout()`, a re-render returns nil.

### T11.12 — RequireLogin redirects

- A route wrapped in `auth.RequireLogin(...)` when unauthenticated renders
  a redirect to `/login`.
- When authenticated, it renders the children.

### T11.13 — Browser E2E

- In chromedp:
  1. Visit `/`, get redirected to `/login`.
  2. Log in with valid credentials.
  3. Verify redirect to `/` and visible username.
  4. Log out and verify redirect back to `/login`.
- Gated `//go:build e2e`.

## Acceptance criteria

- All T11.1–T11.12 pass under `go test`.
- T11.13 passes in the `e2e` job.
- `pkg/auth` coverage >= 80%.
- `docs/auth.md` documents config options, cookie flags, CSRF, session
  stores, and the `RequireLogin` flow.
- Secure defaults are active when `Config.Env="prod"`.
