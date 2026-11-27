package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Hopetree/spool/internal/audit"
	"github.com/Hopetree/spool/internal/envars"
)

type envarInput struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Sensitive   bool   `json:"sensitive"`
}

func (in envarInput) toServiceInput() envars.Input {
	return envars.Input{
		Name: in.Name, Value: in.Value, Description: in.Description, Sensitive: in.Sensitive,
	}
}

func writeEnvarError(c *gin.Context, err error) {
	var ve *envars.ValidationError
	switch {
	case errors.As(err, &ve):
		fail(c, http.StatusBadRequest, "validation_failed", ve.Message, nil)
	case errors.Is(err, envars.ErrNotFound):
		fail(c, http.StatusNotFound, "not_found", "环境变量不存在", nil)
	default:
		fail(c, http.StatusInternalServerError, "internal_error", "操作失败，请稍后重试", nil)
	}
}

func listEnvarsHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := d.Envars.List(c.Request.Context())
		if err != nil {
			writeEnvarError(c, err)
			return
		}
		ok(c, gin.H{"items": items, "total": len(items)})
	}
}

func createEnvarHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in envarInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请求格式不正确", nil)
			return
		}
		u := currentUser(c)
		created, err := d.Envars.Create(c.Request.Context(), in.toServiceInput(), u.ID)
		if err != nil {
			writeEnvarError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "envar.create",
			ResourceType: "platform_env", ResourceID: created.ID, ResourceName: created.Name,
			Summary: map[string]any{"sensitive": created.Sensitive},
		})
		ok(c, created)
	}
}

func updateEnvarHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in envarInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请求格式不正确", nil)
			return
		}
		u := currentUser(c)
		updated, err := d.Envars.Update(c.Request.Context(), c.Param("id"), in.toServiceInput(), u.ID)
		if err != nil {
			writeEnvarError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "envar.update",
			ResourceType: "platform_env", ResourceID: updated.ID, ResourceName: updated.Name,
			Summary: map[string]any{"sensitive": updated.Sensitive},
		})
		ok(c, updated)
	}
}

func deleteEnvarHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		existing, err := d.Envars.Get(c.Request.Context(), id)
		if err != nil {
			writeEnvarError(c, err)
			return
		}
		u := currentUser(c)
		if err := d.Envars.Delete(c.Request.Context(), id); err != nil {
			writeEnvarError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "envar.delete",
			ResourceType: "platform_env", ResourceID: id, ResourceName: existing.Name,
		})
		c.Status(http.StatusNoContent)
	}
}
