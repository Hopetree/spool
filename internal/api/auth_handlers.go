package api

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Hopetree/spool/internal/audit"
	"github.com/Hopetree/spool/internal/auth"
)

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

type userDTO struct {
	ID                 string `json:"id"`
	Username           string `json:"username"`
	DisplayName        string `json:"display_name"`
	Role               string `json:"role"`
	MustChangePassword bool   `json:"must_change_password"`
}

func toUserDTO(u *auth.User) userDTO {
	return userDTO{
		ID:                 u.ID,
		Username:           u.Username,
		DisplayName:        u.DisplayName,
		Role:               u.Role,
		MustChangePassword: u.MustChangePassword,
	}
}

func clientIP(c *gin.Context) string {
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return host
}

func writeAudit(c *gin.Context, d Deps, e audit.Entry) {
	e.IP = clientIP(c)
	e.UserAgent = c.Request.UserAgent()
	if err := d.Audit.Write(c.Request.Context(), e); err != nil {
		slog.Error("audit write failed", "action", e.Action, "err", err)
	}
}

func loginHandler(d Deps, limiter *loginLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req loginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请填写用户名和密码", nil)
			return
		}
		key := clientIP(c) + "|" + req.Username
		if !limiter.Allow(key) {
			fail(c, http.StatusTooManyRequests, "too_many_attempts", "尝试次数过多，请稍后再试", nil)
			return
		}

		u, err := d.Auth.Authenticate(c.Request.Context(), req.Username, req.Password)
		if err != nil {
			limiter.Fail(key)
			reason := "invalid_credentials"
			if errors.Is(err, auth.ErrUserDisabled) {
				reason = "disabled"
			}
			writeAudit(c, d, audit.Entry{
				ActorName: req.Username,
				Action:    "login_failed",
				Summary:   map[string]any{"reason": reason},
			})
			fail(c, http.StatusUnauthorized, "invalid_credentials", "用户名或密码错误", nil)
			return
		}
		limiter.Reset(key)

		ttl := time.Duration(d.Cfg.Security.SessionTTLHours) * time.Hour
		token, err := d.Auth.CreateSession(c.Request.Context(), u.ID, clientIP(c), c.Request.UserAgent())
		if err != nil {
			slog.Error("create session", "err", err)
			fail(c, http.StatusInternalServerError, "internal_error", "登录失败，请稍后重试", nil)
			return
		}
		setSessionCookie(c, d.Cfg, token, ttl)
		writeAudit(c, d, audit.Entry{
			ActorID:   u.ID,
			ActorName: u.Username,
			Action:    "login",
			Summary:   map[string]any{"role": u.Role},
		})
		ok(c, toUserDTO(u))
	}
}

func logoutHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if u := currentUser(c); u != nil {
			writeAudit(c, d, audit.Entry{ActorID: u.ID, ActorName: u.Username, Action: "logout"})
		}
		if token := sessionToken(c); token != "" {
			if err := d.Auth.DeleteSession(c.Request.Context(), token); err != nil {
				slog.Error("delete session", "err", err)
			}
		}
		clearSessionCookie(c, d.Cfg)
		c.Status(http.StatusNoContent)
	}
}

func meHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		if u == nil {
			fail(c, http.StatusUnauthorized, "unauthorized", "请先登录", nil)
			return
		}
		ok(c, toUserDTO(u))
	}
}

func changePasswordHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		if u == nil {
			fail(c, http.StatusUnauthorized, "unauthorized", "请先登录", nil)
			return
		}
		var req changePasswordRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请填写原密码和新密码", nil)
			return
		}
		err := d.Auth.ChangePassword(c.Request.Context(), u.ID, req.OldPassword, req.NewPassword)
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials):
			fail(c, http.StatusBadRequest, "invalid_old_password", "原密码不正确", nil)
			return
		case errors.Is(err, auth.ErrWeakPassword):
			fail(c, http.StatusBadRequest, "weak_password", auth.ErrWeakPassword.Error(), nil)
			return
		case err != nil:
			slog.Error("change password", "err", err)
			fail(c, http.StatusInternalServerError, "internal_error", "修改密码失败", nil)
			return
		}
		if token := sessionToken(c); token != "" {
			if err := d.Auth.DeleteOtherSessions(c.Request.Context(), u.ID, token); err != nil {
				slog.Error("delete other sessions", "err", err)
			}
		}
		writeAudit(c, d, audit.Entry{ActorID: u.ID, ActorName: u.Username, Action: "password_change"})
		c.Status(http.StatusNoContent)
	}
}
