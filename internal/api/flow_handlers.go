package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Hopetree/spool/internal/audit"
	"github.com/Hopetree/spool/internal/flow"
	"github.com/Hopetree/spool/internal/tool"
)

type flowStepInput struct {
	Name          string                  `json:"name"`
	ToolID        string                  `json:"tool_id"`
	Mapping       map[string]flow.Mapping `json:"mapping"`
	TimeoutSec    int                     `json:"timeout_sec"`
	RetryCount    int                     `json:"retry_count"`
	RetryDelaySec int                     `json:"retry_delay_sec"`
	Enabled       *bool                   `json:"enabled"`
}

type flowStageInput struct {
	Name      string          `json:"name"`
	AlwaysRun bool            `json:"always_run"`
	Steps     []flowStepInput `json:"steps"`
}

type flowInput struct {
	Name           string           `json:"name"`
	Description    string           `json:"description"`
	Params         []tool.Param     `json:"params"`
	FailurePolicy  string           `json:"failure_policy"`
	MaxDurationSec int              `json:"max_duration_sec"`
	Stages         []flowStageInput `json:"stages"`
}

func (in flowInput) toServiceInput() flow.Input {
	out := flow.Input{
		Name: in.Name, Description: in.Description, Params: in.Params,
		FailurePolicy: in.FailurePolicy, MaxDurationSec: in.MaxDurationSec,
	}
	for _, stage := range in.Stages {
		si := flow.StageInput{Name: stage.Name, AlwaysRun: stage.AlwaysRun}
		for _, step := range stage.Steps {
			enabled := true
			if step.Enabled != nil {
				enabled = *step.Enabled
			}
			si.Steps = append(si.Steps, flow.StepInput{
				Name: step.Name, ToolID: step.ToolID, Mapping: step.Mapping,
				TimeoutSec: step.TimeoutSec, RetryCount: step.RetryCount,
				RetryDelaySec: step.RetryDelaySec, Enabled: enabled,
			})
		}
		out.Stages = append(out.Stages, si)
	}
	return out
}

func writeFlowError(c *gin.Context, err error) {
	var ve *flow.ValidationError
	switch {
	case errors.As(err, &ve):
		fail(c, http.StatusBadRequest, "validation_failed", ve.Message, nil)
	case errors.Is(err, flow.ErrNotFound):
		fail(c, http.StatusNotFound, "not_found", "流程不存在", nil)
	case errors.Is(err, flow.ErrDisabled):
		fail(c, http.StatusConflict, "flow_disabled", "流程已停用，不能执行", nil)
	default:
		fail(c, http.StatusInternalServerError, "internal_error", "操作失败，请稍后重试", nil)
	}
}

func listFlowsHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		f := flow.ListFilter{Keyword: c.Query("keyword")}
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
		items, total, err := d.Flows.List(c.Request.Context(), f)
		if err != nil {
			writeFlowError(c, err)
			return
		}
		ok(c, pageResponse{Items: items, Total: total, Page: f.Page, PageSize: f.PageSize})
	}
}

func getFlowHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		detail, err := d.Flows.Get(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeFlowError(c, err)
			return
		}
		ok(c, detail)
	}
}

func createFlowHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in flowInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请求格式不正确", nil)
			return
		}
		u := currentUser(c)
		created, err := d.Flows.Create(c.Request.Context(), in.toServiceInput(), u.ID)
		if err != nil {
			writeFlowError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "flow.create",
			ResourceType: "flow", ResourceID: created.ID, ResourceName: created.Name,
			Summary: map[string]any{"stages": created.StageCount, "steps": created.StepCount},
		})
		c.JSON(http.StatusCreated, created)
	}
}

func updateFlowHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in flowInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请求格式不正确", nil)
			return
		}
		u := currentUser(c)
		updated, err := d.Flows.Update(c.Request.Context(), c.Param("id"), in.toServiceInput())
		if err != nil {
			writeFlowError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "flow.update",
			ResourceType: "flow", ResourceID: updated.ID, ResourceName: updated.Name,
			Summary: map[string]any{"stages": updated.StageCount, "steps": updated.StepCount},
		})
		ok(c, updated)
	}
}

func setFlowEnabledHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in enabledInput
		if err := c.ShouldBindJSON(&in); err != nil || in.Enabled == nil {
			fail(c, http.StatusBadRequest, "validation_failed", "缺少 enabled 字段", nil)
			return
		}
		u := currentUser(c)
		if err := d.Flows.SetEnabled(c.Request.Context(), c.Param("id"), *in.Enabled); err != nil {
			writeFlowError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "flow.enabled",
			ResourceType: "flow", ResourceID: c.Param("id"),
			Summary: map[string]any{"enabled": *in.Enabled},
		})
		c.Status(http.StatusNoContent)
	}
}

func deleteFlowHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		if err := d.Flows.Delete(c.Request.Context(), c.Param("id")); err != nil {
			writeFlowError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "flow.delete",
			ResourceType: "flow", ResourceID: c.Param("id"),
		})
		c.Status(http.StatusNoContent)
	}
}

func copyFlowHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		detail, err := d.Flows.Copy(c.Request.Context(), c.Param("id"), u.ID)
		if err != nil {
			writeFlowError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "flow.copy",
			ResourceType: "flow", ResourceID: detail.ID, ResourceName: detail.Name,
			Summary: map[string]any{"source": c.Param("id")},
		})
		c.JSON(http.StatusCreated, detail)
	}
}
