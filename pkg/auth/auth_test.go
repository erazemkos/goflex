package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/erazemkos/goflex/pkg/auth/sessionstore"
	"github.com/erazemkos/goflex/pkg/router"
	"github.com/erazemkos/goflex/pkg/ui"
)

func TestPasswordHash(t *testing.T) {
	h, err := HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$") {
		t.Fatalf("hash prefix=%q", h)
	}
	if !ComparePassword(h, "secret") {
		t.Fatal("want match")
	}
	if ComparePassword(h, "wrong") {
		t.Fatal("wrong matched")
	}
	h2, err := HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	if h == h2 {
		t.Fatal("salt not random")
	}
	if ComparePassword("not-a-hash", "secret") {
		t.Fatal("invalid hash matched")
	}
}

func TestLoginIssuesSessionCookie(t *testing.T) {
	a := NewAuth(Config{CookieName: "sid", CookieSameSite: http.SameSiteStrictMode})
	r := gin.New()
	r.POST("/api/auth/login", a.LoginHandler(func(cr Credentials) (string, bool) {
		return "u1", cr.Email == "a@example.com" && cr.Password == "secret"
	}))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"a@example.com","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	cookie := findCookie(rr.Result().Cookies(), "sid")
	if cookie == nil || cookie.Value == "" || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie=%#v set-cookie=%s", cookie, rr.Header().Values("Set-Cookie"))
	}
}

func TestMiddlewareAttachesUserAndRequireUser(t *testing.T) {
	store := sessionstore.NewMemory()
	a := NewAuth(Config{CookieName: "sid", Store: store, UserLoader: func(ctx context.Context, id string) (User, error) {
		return User{ID: id, Email: id + "@example.com", Name: "Ada"}, nil
	}})
	r := gin.New()
	r.Use(a.Middleware())
	r.GET("/api/me", a.RequireUser(), func(c *gin.Context) { c.JSON(http.StatusOK, CurrentUser(c)) })

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/me", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status=%d body=%s", rr.Code, rr.Body.String())
	}

	cookie := loginCookie(t, a, "u1")
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(cookie)
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("auth status=%d body=%s", rr.Code, rr.Body.String())
	}
	var u User
	if err := json.Unmarshal(rr.Body.Bytes(), &u); err != nil {
		t.Fatal(err)
	}
	if u.ID != "u1" || u.Name != "Ada" {
		t.Fatalf("user=%+v", u)
	}
}

func TestLogoutClearsCookieAndDeletesSession(t *testing.T) {
	store := sessionstore.NewMemory()
	a := NewAuth(Config{CookieName: "sid", Store: store})
	cookie := loginCookie(t, a, "u1")

	r := gin.New()
	r.Use(a.Middleware())
	r.POST("/api/auth/logout", a.LogoutHandler())
	r.GET("/api/me", a.RequireUser(), func(c *gin.Context) { c.JSON(http.StatusOK, CurrentUser(c)) })

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(cookie)
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("logout status=%d", rr.Code)
	}
	cleared := findCookie(rr.Result().Cookies(), "sid")
	if cleared == nil || cleared.MaxAge >= 0 {
		t.Fatalf("clear cookie=%#v", cleared)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(cookie)
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("after logout status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSessionExpirationAndTouch(t *testing.T) {
	store := sessionstore.NewMemory()
	a := NewAuth(Config{CookieName: "sid", Store: store, SessionDuration: time.Hour})
	old := sessionstore.Session{ID: "old", UserID: "u1", ExpiresAt: time.Now().Add(-time.Minute)}
	if err := store.Set(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(a.Middleware())
	r.GET("/api/me", a.RequireUser(), func(c *gin.Context) { c.JSON(http.StatusOK, CurrentUser(c)) })
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: "sid", Value: "old"})
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expired status=%d", rr.Code)
	}

	exp := time.Now().Add(5 * time.Minute)
	active := sessionstore.Session{ID: "active", UserID: "u1", ExpiresAt: exp}
	if err := store.Set(context.Background(), active); err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: "sid", Value: "active"})
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("active status=%d", rr.Code)
	}
	touched, err := store.Get(context.Background(), "active")
	if err != nil {
		t.Fatal(err)
	}
	if !touched.ExpiresAt.After(exp) {
		t.Fatalf("session was not touched: old=%s new=%s", exp, touched.ExpiresAt)
	}
}

func TestCSRFEnforcement(t *testing.T) {
	a := NewAuth(Config{})
	r := gin.New()
	r.Use(a.CSRFMiddleware())
	r.GET("/api/todos", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	r.POST("/api/todos", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/todos", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status=%d", rr.Code)
	}
	csrf := findCookie(rr.Result().Cookies(), "csrf_token")
	if csrf == nil || csrf.Value == "" {
		t.Fatalf("missing csrf cookie: %v", rr.Header().Values("Set-Cookie"))
	}

	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/todos", nil))
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "csrf_failed") {
		t.Fatalf("POST without csrf = %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/todos", nil)
	req.AddCookie(csrf)
	req.Header.Set("X-CSRF-Token", csrf.Value)
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST with csrf = %d %s", rr.Code, rr.Body.String())
	}
}

func TestCookieSessionStoreRejectsTampering(t *testing.T) {
	store := sessionstore.NewCookie([]byte("test-secret"))
	a := NewAuth(Config{CookieName: "sid", Store: store})
	cookie := loginCookie(t, a, "u1")
	r := gin.New()
	r.Use(a.Middleware())
	r.GET("/api/me", a.RequireUser(), func(c *gin.Context) { c.JSON(http.StatusOK, CurrentUser(c)) })

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(cookie)
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("valid cookie status=%d body=%s", rr.Code, rr.Body.String())
	}

	bad := *cookie
	bad.Value += "x"
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(&bad)
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("tampered cookie status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRateLimitedLoginEndpoint(t *testing.T) {
	current := time.Unix(0, 0)
	oldNow := limiterNow
	limiterNow = func() time.Time { return current }
	defer func() { limiterNow = oldNow }()
	a := NewAuth(Config{CookieName: "sid"})
	r := gin.New()
	r.POST("/api/auth/login", a.LoginHandler(func(Credentials) (string, bool) { return "", false }))
	for i := 0; i < 10; i++ {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"a","password":"bad"}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status=%d", i, rr.Code)
		}
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"a","password":"bad"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("limited status=%d body=%s", rr.Code, rr.Body.String())
	}

	current = current.Add(61 * time.Second)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"a","password":"bad"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("after window status=%d", rr.Code)
	}
}

func TestFrontendUseUserAndRequireLogin(t *testing.T) {
	LogoutUser()
	if UseUser() != nil {
		t.Fatal("expected nil user")
	}
	h := router.NewMemoryHistory()
	_ = router.New(router.WithHistory(h))
	guarded := RequireLogin(ui.Text("secret"))
	if len(guarded.Children()) != 0 {
		t.Fatalf("unauthenticated children=%#v", guarded.Children())
	}
	if len(h.Calls) != 1 || h.Calls[0].Kind != "replaceState" || h.Calls[0].Path != "/login" {
		t.Fatalf("redirect calls=%+v", h.Calls)
	}
	u := &User{ID: "u1", Email: "u@example.com"}
	SetCurrentUser(u)
	if UseUser() != u {
		t.Fatal("UseUser did not return inlined user")
	}
	guarded = RequireLogin(ui.Text("secret"))
	if len(guarded.Children()) != 1 || guarded.Children()[0].TextValue() != "secret" {
		t.Fatalf("authenticated guarded=%#v", guarded.Children())
	}
	LogoutUser()
	if UseUser() != nil {
		t.Fatal("LogoutUser should clear user")
	}
}

func TestProductionSecureDefault(t *testing.T) {
	a := NewAuth(Config{Env: "prod"})
	if !a.cfg.CookieSecure {
		t.Fatal("prod should default to secure cookies")
	}
}

func loginCookie(t *testing.T, a *Auth, userID string) *http.Cookie {
	t.Helper()
	r := gin.New()
	r.POST("/login", func(c *gin.Context) { a.Login(c, userID); c.Status(http.StatusOK) })
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/login", nil))
	cookie := findCookie(rr.Result().Cookies(), a.cfg.CookieName)
	if cookie == nil {
		t.Fatalf("missing %s cookie: %v", a.cfg.CookieName, rr.Header().Values("Set-Cookie"))
	}
	return cookie
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}
