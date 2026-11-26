package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Hopetree/spool/internal/audit"
	"github.com/Hopetree/spool/internal/auth"
)

type userInput struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Password    string `json:"password"`
	Enabled     *bool  `json:"enabled"`
}

func writeUserError(c *gin.Context, err error) {
	var ve *auth.UserValidationError
	switch {
	case errors.As(err, &ve):
		fail(c, http.StatusBadRequest, "validation_failed", ve.Message, nil)
	case errors.Is(err, auth.ErrWeakPassword):
		fail(c, http.StatusBadRequest, "weak_password", auth.ErrWeakPassword.Error(), nil)
	case errors.Is(err, auth.ErrUserNotFound):
		fail(c, http.StatusNotFound, "not_found", "用户不存在", nil)
	case errors.Is(err, auth.ErrUserExists):
		fail(c, http.StatusConflict, "user_exists", "用户名已存在", nil)
	case errors.Is(err, auth.ErrLastAdmin):
		fail(c, http.StatusConflict, "last_admin", "不能停用、降级或删除最后一个管理员", nil)
	case errors.Is(err, auth.ErrSelfOperation):
		fail(c, http.StatusConflict, "self_operation", "不能对当前登录账号执行该操作", nil)
	default:
		fail(c, http.StatusInternalServerError, "internal_error", "操作失败，请稍后重试", nil)
	}
}

func listUsersHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		users, err := d.Auth.ListUsers(c.Request.Context())
		if err != nil {
			writeUserError(c, err)
			return
		}
		ok(c, gin.H{"items": users})
	}
}

func createUserHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in userInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请求格式不正确", nil)
			return
		}
		u := currentUser(c)
		created, err := d.Auth.CreateUser(c.Request.Context(), in.Username, in.DisplayName, in.Role, in.Password)
		if err != nil {
			writeUserError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "user.create",
			ResourceType: "user", ResourceID: created.ID, ResourceName: created.Username,
			Summary: map[string]any{"role": created.Role},
		})
		c.JSON(http.StatusCreated, created)
	}
}

func updateUserHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in userInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请求格式不正确", nil)
			return
		}
		enabled := true
		if in.Enabled != nil {
			enabled = *in.Enabled
		}
		u := currentUser(c)
		updated, err := d.Auth.UpdateUser(c.Request.Context(), c.Param("id"), in.DisplayName, in.Role, enabled)
		if err != nil {
			writeUserError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "user.update",
			ResourceType: "user", ResourceID: updated.ID, ResourceName: updated.Username,
			Summary: map[string]any{"role": updated.Role, "enabled": updated.Enabled},
		})
		ok(c, updated)
	}
}

type resetPasswordInput struct {
	Password string `json:"password"`
}

func resetUserPasswordHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in resetPasswordInput
		if err := c.ShouldBindJSON(&in); err != nil || in.Password == "" {
			fail(c, http.StatusBadRequest, "validation_failed", "请填写新密码", nil)
			return
		}
		u := currentUser(c)
		if err := d.Auth.ResetPassword(c.Request.Context(), c.Param("id"), in.Password); err != nil {
			writeUserError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "user.reset_password",
			ResourceType: "user", ResourceID: c.Param("id"),
		})
		c.Status(http.StatusNoContent)
	}
}

func logoutAllSessionsHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		if err := d.Auth.LogoutAll(c.Request.Context(), c.Param("id")); err != nil {
			writeUserError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "user.logout_all",
			ResourceType: "user", ResourceID: c.Param("id"),
		})
		c.Status(http.StatusNoContent)
	}
}

func deleteUserHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		if err := d.Auth.DeleteUser(c.Request.Context(), c.Param("id"), u.ID); err != nil {
			writeUserError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "user.delete",
			ResourceType: "user", ResourceID: c.Param("id"),
		})
		c.Status(http.StatusNoContent)
	}
}

func listAuditHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
		f := audit.Filter{
			ActorID:  c.Query("actor_id"),
			Action:   c.Query("action"),
			Page:     page,
			PageSize: pageSize,
		}
		if v := c.Query("from"); v != "" {
			if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
				f.FromTS = ms
			}
		}
		if v := c.Query("to"); v != "" {
			if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
				f.ToTS = ms
			}
		}
		records, total, err := d.Audit.List(c.Request.Context(), f)
		if err != nil {
			fail(c, http.StatusInternalServerError, "internal_error", "查询失败", nil)
			return
		}
		ok(c, pageResponse{Items: records, Total: total, Page: f.Page, PageSize: f.PageSize})
	}
}
