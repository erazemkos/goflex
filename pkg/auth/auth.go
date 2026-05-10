package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/argon2"

	"github.com/goflex/goflex/pkg/auth/sessionstore"
	"github.com/goflex/goflex/pkg/httperr"
	"github.com/goflex/goflex/pkg/router"
	"github.com/goflex/goflex/pkg/ui"
)

type User struct{ ID, Email, Name string }
type Config struct {
	SecretKey                []byte
	CookieName, CookieDomain string
	CookieSecure             bool
	CookieSameSite           http.SameSite
	SessionDuration          time.Duration
	Store                    sessionstore.Store
	UserLoader               func(context.Context, string) (User, error)
	Env                      string
}
type Auth struct{ cfg Config }

const userKey = "goflex.user"

func NewAuth(cfg Config) *Auth {
	if cfg.CookieName == "" {
		cfg.CookieName = "goflex_session"
	}
	if cfg.SessionDuration == 0 {
		cfg.SessionDuration = 24 * time.Hour
	}
	if cfg.CookieSameSite == 0 {
		cfg.CookieSameSite = http.SameSiteLaxMode
	}
	if cfg.Store == nil {
		cfg.Store = sessionstore.NewMemory()
	}
	if cfg.Env == "prod" {
		cfg.CookieSecure = true
	}
	return &Auth{cfg: cfg}
}
func (a *Auth) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		sid, err := c.Cookie(a.cfg.CookieName)
		if err == nil {
			if sess, err := a.cfg.Store.Get(c.Request.Context(), sid); err == nil {
				_ = a.cfg.Store.Touch(c.Request.Context(), sid, time.Now().Add(a.cfg.SessionDuration))
				if a.cfg.UserLoader != nil {
					if u, err := a.cfg.UserLoader(c.Request.Context(), sess.UserID); err == nil {
						c.Set(userKey, u)
					}
				} else {
					c.Set(userKey, User{ID: sess.UserID})
				}
			}
		}
		ensureCSRF(c)
		c.Next()
	}
}
func (a *Auth) RequireUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		if CurrentUser(c) == nil {
			httperr.Write(c, 401, httperr.New("unauthorized", "login required", nil))
			c.Abort()
			return
		}
		c.Next()
	}
}
func (a *Auth) CSRFMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ensureCSRF(c)
		if c.Request.Method == "GET" || c.Request.Method == "HEAD" || c.Request.Method == "OPTIONS" {
			c.Next()
			return
		}
		ck, err := c.Cookie("csrf_token")
		if err != nil || ck == "" || ck != c.GetHeader("X-CSRF-Token") {
			httperr.Write(c, 403, httperr.New("csrf_failed", "csrf token mismatch", nil))
			c.Abort()
			return
		}
		c.Next()
	}
}
func (a *Auth) Login(c *gin.Context, userID string) {
	expires := time.Now().Add(a.cfg.SessionDuration)
	sess := sessionstore.Session{ID: randToken(32), UserID: userID, ExpiresAt: expires}
	cookieValue := sess.ID
	if enc, ok := a.cfg.Store.(interface {
		Encode(sessionstore.Session) (string, error)
	}); ok {
		if v, err := enc.Encode(sess); err == nil {
			cookieValue = v
		}
	} else {
		_ = a.cfg.Store.Set(c.Request.Context(), sess)
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: a.cfg.CookieName, Value: cookieValue, Path: "/", Domain: a.cfg.CookieDomain, HttpOnly: true, Secure: a.cfg.CookieSecure, SameSite: a.cfg.CookieSameSite, Expires: expires})
}
func (a *Auth) Logout(c *gin.Context) {
	if sid, err := c.Cookie(a.cfg.CookieName); err == nil {
		_ = a.cfg.Store.Delete(c.Request.Context(), sid)
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: a.cfg.CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
}
func CurrentUser(c *gin.Context) *User {
	if v, ok := c.Get(userKey); ok {
		if u, ok := v.(User); ok {
			return &u
		}
	}
	return nil
}

func ensureCSRF(c *gin.Context) {
	if _, err := c.Cookie("csrf_token"); err != nil {
		http.SetCookie(c.Writer, &http.Cookie{Name: "csrf_token", Value: randToken(24), Path: "/", SameSite: http.SameSiteLaxMode})
	}
}
func randToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=1,p=4$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}
func ComparePassword(encoded, password string) bool {
	parts := splitDollar(encoded)
	if len(parts) < 6 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}
func splitDollar(s string) []string {
	out := []string{""}
	for _, r := range s {
		if r == '$' {
			out = append(out, "")
		} else {
			out[len(out)-1] += string(r)
		}
	}
	return out
}

var frontendUser *User

func SetCurrentUser(u *User) { frontendUser = u }
func UseUser() *User         { return frontendUser }
func LogoutUser()            { frontendUser = nil }

func RequireLogin(children ...ui.Element) ui.Element {
	if frontendUser == nil {
		router.UseNavigate()("/login", router.Replace())
		return ui.Fragment()
	}
	return ui.Fragment(children...)
}
