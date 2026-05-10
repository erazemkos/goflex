# Auth and Sessions

`pkg/auth` provides email/password-oriented primitives: cookie sessions, user-loading middleware, CSRF protection, route guards, password hashing, and small frontend helpers.

## Config

```go
a := auth.NewAuth(auth.Config{
    Env:             "prod",
    SecretKey:       []byte(os.Getenv("AUTH_SECRET")),
    CookieName:      "goflex_session",
    CookieDomain:    "example.com",
    CookieSecure:    true,
    CookieSameSite:  http.SameSiteLaxMode,
    SessionDuration: 24 * time.Hour,
    Store:           sessionstore.NewMemory(),
    UserLoader: func(ctx context.Context, id string) (auth.User, error) {
        return loadUser(ctx, id)
    },
})
```

Defaults:

- `CookieName`: `goflex_session`
- `SessionDuration`: 24h
- `SameSite`: `Lax`
- `Store`: in-memory dev store
- `CookieSecure`: true when `Env == "prod"`
- Session cookies are `HttpOnly`.

## Gin middleware

```go
r := gin.New()
r.Use(a.Middleware())      // attaches auth.CurrentUser(c), sets CSRF cookie
r.Use(a.CSRFMiddleware())  // checks X-CSRF-Token for mutating requests

r.GET("/api/me", a.RequireUser(), func(c *gin.Context) {
    c.JSON(200, auth.CurrentUser(c))
})
```

`RequireUser` returns a JSON `401 {"code":"unauthorized"...}` envelope when no valid session exists.

## Login and logout

```go
r.POST("/api/auth/login", a.LoginHandler(func(c auth.Credentials) (string, bool) {
    user, ok := findUserByEmail(c.Email)
    if !ok || !auth.ComparePassword(user.PasswordHash, c.Password) {
        return "", false
    }
    return user.ID, true
}))
r.POST("/api/auth/logout", a.LogoutHandler())
```

`Login` issues a session cookie. `Logout` deletes the server-side session when applicable and clears the browser cookie.

The login handler includes a per-IP failure limiter: 10 failed attempts per 60 seconds return `429 rate_limited` until the window expires. Successful login resets the counter.

## Passwords

```go
hash, err := auth.HashPassword("secret")
ok := auth.ComparePassword(hash, "secret")
```

Hashes use Argon2id with a random salt, so repeated calls produce different strings.

## CSRF

GoFlex uses a double-submit token:

1. Middleware sets a readable `csrf_token` cookie.
2. Frontend clients send `X-CSRF-Token: <cookie value>` for POST/PUT/PATCH/DELETE.
3. The server compares cookie and header; mismatches return `403 csrf_failed`.

GET, HEAD, and OPTIONS are exempt.

## Session stores

- `sessionstore.NewMemory()` — in-process dev/testing store with sliding expiration.
- `sessionstore.NewCookie(secret)` — stateless signed cookie store. Tampering rejects the session.
- `sessionstore.NewGORM(db)` — DB-backed sessions with `Get`, `Set`, `Delete`, `Touch`, and `Cleanup(ctx)` for expired rows.

## Frontend helpers

```go
user := auth.UseUser()

auth.SetCurrentUser(&auth.User{ID: "1", Email: "ada@example.com"}) // bootstrap window.__USER__ equivalent
auth.LogoutUser()

page := auth.RequireLogin(ui.Text("private"))
```

`RequireLogin` redirects unauthenticated users to `/login` using the frontend router and renders children when `UseUser()` is non-nil.
