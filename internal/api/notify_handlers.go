package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Hopetree/spool/internal/audit"
	"github.com/Hopetree/spool/internal/notify"
)

type channelInput struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	URL     string `json:"url"`
	Secret  string `json:"secret"`
	Enabled *bool  `json:"enabled"`
}

func (in channelInput) toServiceInput() notify.Input {
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	return notify.Input{Name: in.Name, Type: in.Type, URL: in.URL, Secret: in.Secret, Enabled: enabled}
}

func writeChannelError(c *gin.Context, err error) {
	var ve *notify.ValidationError
	switch {
	case errors.As(err, &ve):
		fail(c, http.StatusBadRequest, "validation_failed", ve.Message, nil)
	case errors.Is(err, notify.ErrNotFound):
		fail(c, http.StatusNotFound, "not_found", "通知渠道不存在", nil)
	default:
		fail(c, http.StatusInternalServerError, "internal_error", "操作失败，请稍后重试", nil)
	}
}

func listChannelsHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := d.Notify.List(c.Request.Context())
		if err != nil {
			writeChannelError(c, err)
			return
		}
		ok(c, gin.H{"items": items})
	}
}

func createChannelHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in channelInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请求格式不正确", nil)
			return
		}
		u := currentUser(c)
		created, err := d.Notify.Create(c.Request.Context(), in.toServiceInput())
		if err != nil {
			writeChannelError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "notify.create",
			ResourceType: "notify_channel", ResourceID: created.ID, ResourceName: created.Name,
			Summary: map[string]any{"type": created.Type},
		})
		c.JSON(http.StatusCreated, created)
	}
}

func updateChannelHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in channelInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请求格式不正确", nil)
			return
		}
		u := currentUser(c)
		updated, err := d.Notify.Update(c.Request.Context(), c.Param("id"), in.toServiceInput())
		if err != nil {
			writeChannelError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "notify.update",
			ResourceType: "notify_channel", ResourceID: updated.ID, ResourceName: updated.Name,
			Summary: map[string]any{"type": updated.Type, "enabled": updated.Enabled},
		})
		ok(c, updated)
	}
}

func deleteChannelHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		if err := d.Notify.Delete(c.Request.Context(), c.Param("id")); err != nil {
			writeChannelError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "notify.delete",
			ResourceType: "notify_channel", ResourceID: c.Param("id"),
		})
		c.Status(http.StatusNoContent)
	}
}

func testChannelHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		resp, err := d.Notify.Test(c.Request.Context(), c.Param("id"))
		if err != nil {
			var ve *notify.ValidationError
			if errors.As(err, &ve) {
				fail(c, http.StatusBadRequest, "validation_failed", ve.Message, nil)
				return
			}
			if errors.Is(err, notify.ErrNotFound) {
				fail(c, http.StatusNotFound, "not_found", "通知渠道不存在", nil)
				return
			}
			fail(c, http.StatusBadGateway, "notify_failed", err.Error(), nil)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "notify.test",
			ResourceType: "notify_channel", ResourceID: c.Param("id"),
		})
		ok(c, gin.H{"ok": true, "response": resp})
	}
}
