package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Hopetree/spool/internal/audit"
	"github.com/Hopetree/spool/internal/tool"
)

type toolInput struct {
	Name           string       `json:"name"`
	Description    string       `json:"description"`
	Type           string       `json:"type"`
	Interpreter    string       `json:"interpreter"`
	Content        string       `json:"content"`
	PrepareCommand string       `json:"prepare_command"`
	Params         []tool.Param `json:"params"`
	TimeoutSec     int          `json:"timeout_sec"`
	Sudo           bool         `json:"sudo"`
	ChangeNote     string       `json:"change_note"`
}

func (in toolInput) toServiceInput() tool.Input {
	return tool.Input{
		Name:           in.Name,
		Description:    in.Description,
		Type:           in.Type,
		Interpreter:    in.Interpreter,
		Content:        in.Content,
		PrepareCommand: in.PrepareCommand,
		Params:         in.Params,
		TimeoutSec:     in.TimeoutSec,
		Sudo:           in.Sudo,
		ChangeNote:     in.ChangeNote,
	}
}

func writeToolError(c *gin.Context, err error) {
	var ve *tool.ValidationError
	switch {
	case errors.As(err, &ve):
		fail(c, http.StatusBadRequest, "validation_failed", ve.Message, nil)
	case errors.Is(err, tool.ErrNotFound):
		fail(c, http.StatusNotFound, "not_found", "工具不存在", nil)
	case errors.Is(err, tool.ErrHasHistory):
		fail(c, http.StatusConflict, "tool_has_history", "工具已有执行历史，只能停用", nil)
	default:
		fail(c, http.StatusInternalServerError, "internal_error", "操作失败，请稍后重试", nil)
	}
}

func listToolsHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		f := tool.ListFilter{Keyword: c.Query("keyword")}
		f.Page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
		f.PageSize, _ = strconv.Atoi(c.DefaultQuery("page_size", "20"))
		if f.Page < 1 {
			f.Page = 1
		}
		if f.PageSize < 1 || f.PageSize > 100 {
			f.PageSize = 20
		}
		if v := c.Query("enabled"); v != "" {
			if b, err := strconv.ParseBool(v); err == nil {
				f.Enabled = &b
			}
		}
		items, total, err := d.Tools.List(c.Request.Context(), f)
		if err != nil {
			writeToolError(c, err)
			return
		}
		ok(c, pageResponse{Items: items, Total: total, Page: f.Page, PageSize: f.PageSize})
	}
}

func getToolHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		detail, err := d.Tools.Get(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeToolError(c, err)
			return
		}
		ok(c, detail)
	}
}

func createToolHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in toolInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请求格式不正确", nil)
			return
		}
		u := currentUser(c)
		created, err := d.Tools.Create(c.Request.Context(), in.toServiceInput(), u.ID)
		if err != nil {
			writeToolError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "tool.create",
			ResourceType: "tool", ResourceID: created.ID, ResourceName: created.Name,
			Summary: map[string]any{"type": created.Version.Type, "version": created.VersionNo},
		})
		c.JSON(http.StatusCreated, created)
	}
}

func updateToolHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in toolInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请求格式不正确", nil)
			return
		}
		u := currentUser(c)
		updated, err := d.Tools.Update(c.Request.Context(), c.Param("id"), in.toServiceInput(), u.ID)
		if err != nil {
			writeToolError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "tool.update",
			ResourceType: "tool", ResourceID: updated.ID, ResourceName: updated.Name,
			Summary: map[string]any{"version": updated.VersionNo},
		})
		ok(c, updated)
	}
}

type enabledInput struct {
	Enabled *bool `json:"enabled"`
}

func setToolEnabledHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in enabledInput
		if err := c.ShouldBindJSON(&in); err != nil || in.Enabled == nil {
			fail(c, http.StatusBadRequest, "validation_failed", "缺少 enabled 字段", nil)
			return
		}
		u := currentUser(c)
		if err := d.Tools.SetEnabled(c.Request.Context(), c.Param("id"), *in.Enabled); err != nil {
			writeToolError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "tool.enabled",
			ResourceType: "tool", ResourceID: c.Param("id"),
			Summary: map[string]any{"enabled": *in.Enabled},
		})
		c.Status(http.StatusNoContent)
	}
}

func deleteToolHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		if err := d.Tools.Delete(c.Request.Context(), c.Param("id")); err != nil {
			writeToolError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "tool.delete",
			ResourceType: "tool", ResourceID: c.Param("id"),
		})
		c.Status(http.StatusNoContent)
	}
}

func toolVersionsHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := d.Tools.Versions(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeToolError(c, err)
			return
		}
		ok(c, gin.H{"items": items})
	}
}

func toolVersionHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		no, err := strconv.Atoi(c.Param("no"))
		if err != nil || no <= 0 {
			fail(c, http.StatusBadRequest, "validation_failed", "版本号不正确", nil)
			return
		}
		v, err := d.Tools.Version(c.Request.Context(), c.Param("id"), no)
		if err != nil {
			writeToolError(c, err)
			return
		}
		ok(c, v)
	}
}

type rollbackInput struct {
	VersionNo int    `json:"version_no"`
	Note      string `json:"note"`
}

func rollbackToolHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in rollbackInput
		if err := c.ShouldBindJSON(&in); err != nil || in.VersionNo <= 0 {
			fail(c, http.StatusBadRequest, "validation_failed", "请提供版本号", nil)
			return
		}
		u := currentUser(c)
		detail, err := d.Tools.Rollback(c.Request.Context(), c.Param("id"), in.VersionNo, in.Note, u.ID)
		if err != nil {
			writeToolError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "tool.rollback",
			ResourceType: "tool", ResourceID: detail.ID, ResourceName: detail.Name,
			Summary: map[string]any{"from": in.VersionNo, "new_version": detail.VersionNo},
		})
		ok(c, detail)
	}
}

func copyToolHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		detail, err := d.Tools.Copy(c.Request.Context(), c.Param("id"), u.ID)
		if err != nil {
			writeToolError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "tool.copy",
			ResourceType: "tool", ResourceID: detail.ID, ResourceName: detail.Name,
			Summary: map[string]any{"source": c.Param("id")},
		})
		c.JSON(http.StatusCreated, detail)
	}
}
