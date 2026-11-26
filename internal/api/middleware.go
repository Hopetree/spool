package api

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Hopetree/spool/internal/auth"
	"github.com/Hopetree/spool/internal/config"
)

const (
	// SessionCookieName is the HttpOnly cookie carrying the session token.
	SessionCookieName = "spool_session"
	ctxUserKey        = "spool.user"
)

func currentUser(c *gin.Context) *auth.User {
	v, ok := c.Get(ctxUserKey)
	if !ok {
		return nil
	}
	u, _ := v.(*auth.User)
	return u
}

func setSessionCookie(c *gin.Context, cfg *config.Config, token string, ttl time.Duration) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   cfg.Server.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

func clearSessionCookie(c *gin.Context, cfg *config.Config) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   cfg.Server.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func sessionToken(c *gin.Context) string {
	token, err := c.Cookie(SessionCookieName)
	if err != nil {
		return ""
	}
	return token
}

// SecurityHeaders sets baseline browser hardening headers.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")
		c.Next()
	}
}

// OriginCheck rejects mutating requests whose Origin does not match the host.
func OriginCheck(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			fail(c, http.StatusForbidden, "cross_origin_denied", "跨站请求被拒绝", nil)
			return
		}
		if strings.EqualFold(u.Host, c.Request.Host) {
			c.Next()
			return
		}
		if cfg.Server.BaseURL != "" {
			if base, err := url.Parse(cfg.Server.BaseURL); err == nil && strings.EqualFold(base.Host, u.Host) {
				c.Next()
				return
			}
		}
		fail(c, http.StatusForbidden, "cross_origin_denied", "跨站请求被拒绝", nil)
	}
}

// RequireAuth resolves the session cookie and injects the user into context.
func RequireAuth(svc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := sessionToken(c)
		if token == "" {
			fail(c, http.StatusUnauthorized, "unauthorized", "请先登录", nil)
			return
		}
		u, err := svc.UserBySessionToken(c.Request.Context(), token)
		if err != nil {
			fail(c, http.StatusUnauthorized, "unauthorized", "登录已失效，请重新登录", nil)
			return
		}
		c.Set(ctxUserKey, u)
		c.Next()
	}
}

// RequireRole enforces a minimum role. admin satisfies every requirement.
func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		if u == nil {
			fail(c, http.StatusUnauthorized, "unauthorized", "请先登录", nil)
			return
		}
		if u.Role == "admin" || u.Role == role {
			c.Next()
			return
		}
		fail(c, http.StatusForbidden, "forbidden", "权限不足", nil)
	}
}

// ForcePasswordChange blocks users who must change their initial password.
func ForcePasswordChange() gin.HandlerFunc {
	allowed := map[string]bool{
		http.MethodGet + " /api/v1/auth/me":       true,
		http.MethodPut + " /api/v1/auth/password": true,
		http.MethodPost + " /api/v1/auth/logout":  true,
	}
	return func(c *gin.Context) {
		u := currentUser(c)
		if u == nil || !u.MustChangePassword {
			c.Next()
			return
		}
		if allowed[c.Request.Method+" "+c.Request.URL.Path] {
			c.Next()
			return
		}
		fail(c, http.StatusForbidden, "password_change_required", "请先修改初始密码", nil)
	}
}

func requestLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("http request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds())
	}
}
