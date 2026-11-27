package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Hopetree/spool/internal/audit"
	"github.com/Hopetree/spool/internal/credential"
	"github.com/Hopetree/spool/internal/host"
	"github.com/Hopetree/spool/internal/sshx"
)

type hostInput struct {
	Name         string   `json:"name"`
	Address      string   `json:"address"`
	Port         int      `json:"port"`
	GroupID      string   `json:"group_id"`
	CredentialID string   `json:"credential_id"`
	Enabled      *bool    `json:"enabled"`
	Note         string   `json:"note"`
	Tags         []string `json:"tags"`
}

func (in hostInput) toServiceInput() host.Input {
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	return host.Input{
		Name:         in.Name,
		Address:      in.Address,
		Port:         in.Port,
		GroupID:      in.GroupID,
		CredentialID: in.CredentialID,
		Enabled:      enabled,
		Note:         in.Note,
		Tags:         in.Tags,
	}
}

func sanitizeHost(h host.Host, role string) host.Host {
	if role != "admin" {
		h.CredentialID = ""
		h.CredentialName = ""
	}
	return h
}

func writeHostError(c *gin.Context, err error) {
	var ve *host.ValidationError
	switch {
	case errors.As(err, &ve):
		fail(c, http.StatusBadRequest, "validation_failed", ve.Message, nil)
	case errors.Is(err, host.ErrNotFound):
		fail(c, http.StatusNotFound, "not_found", "主机不存在", nil)
	default:
		fail(c, http.StatusInternalServerError, "internal_error", "操作失败，请稍后重试", nil)
	}
}

func writeHostTestError(c *gin.Context, err error) {
	var ve *host.ValidationError
	var keyErr *sshx.HostKeyError
	switch {
	case errors.As(err, &keyErr):
		fail(c, http.StatusConflict, "host_key_mismatch", "主机指纹未知或已变化，请确认后更新指纹", gin.H{
			"expected": keyErr.Expected,
			"actual":   keyErr.Actual,
			"key_type": keyErr.KeyType,
			"changed":  keyErr.Changed,
		})
	case errors.As(err, &ve):
		fail(c, http.StatusBadRequest, "validation_failed", ve.Message, nil)
	case errors.Is(err, host.ErrNotFound):
		fail(c, http.StatusNotFound, "not_found", "主机不存在", nil)
	case errors.Is(err, credential.ErrMasterKeyMismatch):
		fail(c, http.StatusInternalServerError, "master_key_mismatch", "主密钥与凭据不匹配，请检查 master.key", nil)
	default:
		fail(c, http.StatusBadGateway, "ssh_error", err.Error(), nil)
	}
}

func listHostsHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		f := host.Filter{
			GroupID: c.Query("group_id"),
			Tag:     c.Query("tag"),
			Keyword: c.Query("keyword"),
		}
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
		items, total, err := d.Hosts.List(c.Request.Context(), f)
		if err != nil {
			writeHostError(c, err)
			return
		}
		role := currentUser(c).Role
		for i := range items {
			items[i] = sanitizeHost(items[i], role)
		}
		ok(c, pageResponse{Items: items, Total: total, Page: f.Page, PageSize: f.PageSize})
	}
}

func getHostHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		h, err := d.Hosts.Get(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeHostError(c, err)
			return
		}
		ok(c, sanitizeHost(*h, currentUser(c).Role))
	}
}

func createHostHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in hostInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请求格式不正确", nil)
			return
		}
		u := currentUser(c)
		created, err := d.Hosts.Create(c.Request.Context(), in.toServiceInput())
		if err != nil {
			writeHostError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "host.create",
			ResourceType: "host", ResourceID: created.ID, ResourceName: created.Name,
			Summary: map[string]any{"address": created.Address, "port": created.Port},
		})
		c.JSON(http.StatusCreated, sanitizeHost(*created, u.Role))
	}
}

type hostImportRow struct {
	Name    string   `json:"name"`
	Address string   `json:"address"`
	Port    int      `json:"port"`
	Tags    []string `json:"tags"`
	Note    string   `json:"note"`
	Enabled *bool    `json:"enabled"`
}

type hostImportInput struct {
	GroupID      string          `json:"group_id"`
	CredentialID string          `json:"credential_id"`
	Rows         []hostImportRow `json:"rows"`
}

// importHostsHandler creates hosts from a parsed spreadsheet. Rows are
// independent: invalid or duplicate rows are reported without aborting the
// rest of the batch.
func importHostsHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in hostImportInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请求格式不正确", nil)
			return
		}
		if strings.TrimSpace(in.CredentialID) == "" {
			fail(c, http.StatusBadRequest, "validation_failed", "请选择登录凭据", nil)
			return
		}
		if len(in.Rows) == 0 {
			fail(c, http.StatusBadRequest, "validation_failed", "导入数据为空", nil)
			return
		}
		if len(in.Rows) > 500 {
			fail(c, http.StatusBadRequest, "validation_failed", "单次最多导入 500 台主机", nil)
			return
		}
		u := currentUser(c)
		type rowError struct {
			Row     int    `json:"row"`
			Message string `json:"message"`
		}
		failures := make([]rowError, 0)
		created := 0
		for i, row := range in.Rows {
			enabled := true
			if row.Enabled != nil {
				enabled = *row.Enabled
			}
			_, err := d.Hosts.Create(c.Request.Context(), host.Input{
				Name:         strings.TrimSpace(row.Name),
				Address:      strings.TrimSpace(row.Address),
				Port:         row.Port,
				GroupID:      in.GroupID,
				CredentialID: in.CredentialID,
				Enabled:      enabled,
				Note:         row.Note,
				Tags:         row.Tags,
			})
			if err != nil {
				failures = append(failures, rowError{Row: i + 2, Message: err.Error()})
				continue
			}
			created++
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "host.import",
			ResourceType: "host", ResourceName: "批量导入",
			Summary: map[string]any{"created": created, "failed": len(failures), "group_id": in.GroupID},
		})
		ok(c, gin.H{"created": created, "failed": len(failures), "errors": failures})
	}
}

func updateHostHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in hostInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请求格式不正确", nil)
			return
		}
		u := currentUser(c)
		updated, err := d.Hosts.Update(c.Request.Context(), c.Param("id"), in.toServiceInput())
		if err != nil {
			writeHostError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "host.update",
			ResourceType: "host", ResourceID: updated.ID, ResourceName: updated.Name,
			Summary: map[string]any{"address": updated.Address, "enabled": updated.Enabled},
		})
		ok(c, sanitizeHost(*updated, u.Role))
	}
}

func deleteHostHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		id := c.Param("id")
		hostName := ""
		if h, err := d.Hosts.Get(c.Request.Context(), id); err == nil {
			hostName = h.Name
		}
		if err := d.Hosts.Delete(c.Request.Context(), id); err != nil {
			writeHostError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "host.delete",
			ResourceType: "host", ResourceID: id, ResourceName: hostName,
		})
		c.Status(http.StatusNoContent)
	}
}

func testHostHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		id := c.Param("id")
		hostName := ""
		if h, err := d.Hosts.Get(c.Request.Context(), id); err == nil {
			hostName = h.Name
		}
		res, err := d.Hosts.Test(c.Request.Context(), id)
		if err != nil {
			summary := map[string]any{"ok": false, "error": err.Error()}
			writeAudit(c, d, audit.Entry{
				ActorID: u.ID, ActorName: u.Username, Action: "host.test",
				ResourceType: "host", ResourceID: id, ResourceName: hostName, Summary: summary,
			})
			writeHostTestError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "host.test",
			ResourceType: "host", ResourceID: id, ResourceName: hostName,
			Summary: map[string]any{"ok": true, "fingerprint_recorded": res.FingerprintRecorded},
		})
		ok(c, res)
	}
}

type fingerprintInput struct {
	Fingerprint string `json:"fingerprint"`
	KeyType     string `json:"key_type"`
}

func setFingerprintHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in fingerprintInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请求格式不正确", nil)
			return
		}
		u := currentUser(c)
		h, err := d.Hosts.SetFingerprint(c.Request.Context(), c.Param("id"), in.Fingerprint, in.KeyType)
		if err != nil {
			writeHostError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "host.fingerprint_update",
			ResourceType: "host", ResourceID: h.ID, ResourceName: h.Name,
			Summary: map[string]any{"fingerprint": h.Fingerprint, "key_type": h.FingerprintType},
		})
		ok(c, sanitizeHost(*h, u.Role))
	}
}

// ---- groups ----

type groupInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (in groupInput) toServiceInput() host.GroupInput {
	return host.GroupInput{Name: in.Name, Description: in.Description}
}

func writeGroupError(c *gin.Context, err error) {
	var ve *host.ValidationError
	switch {
	case errors.As(err, &ve):
		fail(c, http.StatusBadRequest, "validation_failed", ve.Message, nil)
	case errors.Is(err, host.ErrNotFound):
		fail(c, http.StatusNotFound, "not_found", "分组不存在", nil)
	default:
		fail(c, http.StatusInternalServerError, "internal_error", "操作失败，请稍后重试", nil)
	}
}

func listGroupsHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := d.Hosts.ListGroups(c.Request.Context())
		if err != nil {
			writeGroupError(c, err)
			return
		}
		ok(c, gin.H{"items": items})
	}
}

func createGroupHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in groupInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请求格式不正确", nil)
			return
		}
		u := currentUser(c)
		g, err := d.Hosts.CreateGroup(c.Request.Context(), in.toServiceInput())
		if err != nil {
			writeGroupError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "group.create",
			ResourceType: "host_group", ResourceID: g.ID, ResourceName: g.Name,
		})
		c.JSON(http.StatusCreated, g)
	}
}

func updateGroupHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in groupInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请求格式不正确", nil)
			return
		}
		u := currentUser(c)
		g, err := d.Hosts.UpdateGroup(c.Request.Context(), c.Param("id"), in.toServiceInput())
		if err != nil {
			writeGroupError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "group.update",
			ResourceType: "host_group", ResourceID: g.ID, ResourceName: g.Name,
		})
		ok(c, g)
	}
}

func deleteGroupHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		id := c.Param("id")
		if err := d.Hosts.DeleteGroup(c.Request.Context(), id); err != nil {
			writeGroupError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "group.delete",
			ResourceType: "host_group", ResourceID: id,
		})
		c.Status(http.StatusNoContent)
	}
}
