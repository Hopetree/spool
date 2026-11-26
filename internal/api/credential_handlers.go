package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Hopetree/spool/internal/audit"
	"github.com/Hopetree/spool/internal/credential"
)

type credentialInput struct {
	Name        string `json:"name"`
	AuthType    string `json:"auth_type"`
	Username    string `json:"username"`
	Secret      string `json:"secret"`
	Passphrase  string `json:"passphrase"`
	Description string `json:"description"`
}

func (in credentialInput) toServiceInput() credential.Input {
	return credential.Input{
		Name:        in.Name,
		AuthType:    in.AuthType,
		Username:    in.Username,
		Secret:      in.Secret,
		Passphrase:  in.Passphrase,
		Description: in.Description,
	}
}

func writeCredentialError(c *gin.Context, err error) {
	var ve *credential.ValidationError
	var inUse *credential.ErrInUse
	switch {
	case errors.As(err, &ve):
		fail(c, http.StatusBadRequest, "validation_failed", ve.Message, nil)
	case errors.As(err, &inUse):
		fail(c, http.StatusConflict, "credential_in_use", inUse.Error(), gin.H{"hosts": inUse.Hosts})
	case errors.Is(err, credential.ErrNotFound):
		fail(c, http.StatusNotFound, "not_found", "凭据不存在", nil)
	default:
		fail(c, http.StatusInternalServerError, "internal_error", "操作失败，请稍后重试", nil)
	}
}

func listCredentialsHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := d.Credentials.List(c.Request.Context())
		if err != nil {
			writeCredentialError(c, err)
			return
		}
		ok(c, gin.H{"items": items})
	}
}

func createCredentialHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in credentialInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请求格式不正确", nil)
			return
		}
		u := currentUser(c)
		created, err := d.Credentials.Create(c.Request.Context(), in.toServiceInput(), u.ID)
		if err != nil {
			writeCredentialError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "credential.create",
			ResourceType: "credential", ResourceID: created.ID, ResourceName: created.Name,
			Summary: map[string]any{"auth_type": created.AuthType},
		})
		c.JSON(http.StatusCreated, created)
	}
}

func updateCredentialHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in credentialInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请求格式不正确", nil)
			return
		}
		u := currentUser(c)
		updated, err := d.Credentials.Update(c.Request.Context(), c.Param("id"), in.toServiceInput())
		if err != nil {
			writeCredentialError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "credential.update",
			ResourceType: "credential", ResourceID: updated.ID, ResourceName: updated.Name,
			Summary: map[string]any{"auth_type": updated.AuthType},
		})
		ok(c, updated)
	}
}

func deleteCredentialHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		id := c.Param("id")
		if err := d.Credentials.Delete(c.Request.Context(), id); err != nil {
			writeCredentialError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "credential.delete",
			ResourceType: "credential", ResourceID: id,
		})
		c.Status(http.StatusNoContent)
	}
}
