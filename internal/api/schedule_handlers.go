package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Hopetree/spool/internal/audit"
	"github.com/Hopetree/spool/internal/flow"
	"github.com/Hopetree/spool/internal/schedule"
	"github.com/Hopetree/spool/internal/tool"
)

type scheduleInput struct {
	Name              string            `json:"name"`
	TargetKind        string            `json:"target_kind"`
	TargetRefID       string            `json:"target_ref_id"`
	Params            map[string]string `json:"params"`
	HostIDs           []string          `json:"host_ids"`
	CronExpr          string            `json:"cron_expr"`
	ConcurrencyPolicy string            `json:"concurrency_policy"`
	NotifyChannelIDs  []string          `json:"notify_channel_ids"`
	NotifyEvents      string            `json:"notify_events"`
	Enabled           *bool             `json:"enabled"`
}

func (in scheduleInput) toServiceInput() schedule.Input {
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	return schedule.Input{
		Name: in.Name, TargetKind: in.TargetKind, TargetRefID: in.TargetRefID,
		Params: in.Params, HostIDs: in.HostIDs, CronExpr: in.CronExpr,
		ConcurrencyPolicy: in.ConcurrencyPolicy, NotifyChannelIDs: in.NotifyChannelIDs,
		NotifyEvents: in.NotifyEvents, Enabled: enabled,
	}
}

func writeScheduleError(c *gin.Context, err error) {
	var ve *schedule.ValidationError
	var tve *tool.ValidationError
	var fve *flow.ValidationError
	switch {
	case errors.As(err, &ve):
		fail(c, http.StatusBadRequest, "validation_failed", ve.Message, nil)
	case errors.As(err, &tve):
		fail(c, http.StatusBadRequest, "validation_failed", tve.Message, nil)
	case errors.As(err, &fve):
		fail(c, http.StatusBadRequest, "validation_failed", fve.Message, nil)
	case errors.Is(err, schedule.ErrNotFound):
		fail(c, http.StatusNotFound, "not_found", "定时任务不存在", nil)
	case errors.Is(err, flow.ErrDisabled):
		fail(c, http.StatusConflict, "flow_disabled", "流程已停用", nil)
	case errors.Is(err, tool.ErrNotFound):
		fail(c, http.StatusBadRequest, "validation_failed", "工具不存在", nil)
	default:
		fail(c, http.StatusBadGateway, "run_failed", err.Error(), nil)
	}
}

func listSchedulesHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		f := schedule.ListFilter{Keyword: c.Query("keyword")}
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
		items, total, err := d.Schedules.List(c.Request.Context(), f)
		if err != nil {
			writeScheduleError(c, err)
			return
		}
		ok(c, pageResponse{Items: items, Total: total, Page: f.Page, PageSize: f.PageSize})
	}
}

func getScheduleHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		sch, err := d.Schedules.Get(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeScheduleError(c, err)
			return
		}
		ok(c, sch)
	}
}

func createScheduleHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in scheduleInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请求格式不正确", nil)
			return
		}
		u := currentUser(c)
		created, err := d.Schedules.Create(c.Request.Context(), in.toServiceInput(), u.ID)
		if err != nil {
			writeScheduleError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "schedule.create",
			ResourceType: "schedule", ResourceID: created.ID, ResourceName: created.Name,
			Summary: map[string]any{"cron": created.CronExpr, "target_kind": created.TargetKind},
		})
		c.JSON(http.StatusCreated, created)
	}
}

func updateScheduleHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in scheduleInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "请求格式不正确", nil)
			return
		}
		u := currentUser(c)
		updated, err := d.Schedules.Update(c.Request.Context(), c.Param("id"), in.toServiceInput())
		if err != nil {
			writeScheduleError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "schedule.update",
			ResourceType: "schedule", ResourceID: updated.ID, ResourceName: updated.Name,
			Summary: map[string]any{"cron": updated.CronExpr, "enabled": updated.Enabled},
		})
		ok(c, updated)
	}
}

func setScheduleEnabledHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in enabledInput
		if err := c.ShouldBindJSON(&in); err != nil || in.Enabled == nil {
			fail(c, http.StatusBadRequest, "validation_failed", "缺少 enabled 字段", nil)
			return
		}
		u := currentUser(c)
		if err := d.Schedules.SetEnabled(c.Request.Context(), c.Param("id"), *in.Enabled); err != nil {
			writeScheduleError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "schedule.enabled",
			ResourceType: "schedule", ResourceID: c.Param("id"),
			Summary: map[string]any{"enabled": *in.Enabled},
		})
		c.Status(http.StatusNoContent)
	}
}

func deleteScheduleHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		if err := d.Schedules.Delete(c.Request.Context(), c.Param("id")); err != nil {
			writeScheduleError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "schedule.delete",
			ResourceType: "schedule", ResourceID: c.Param("id"),
		})
		c.Status(http.StatusNoContent)
	}
}

func runScheduleHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		ex, err := d.Schedules.RunNow(c.Request.Context(), c.Param("id"), u.ID)
		if err != nil {
			writeScheduleError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "schedule.trigger",
			ResourceType: "schedule", ResourceID: c.Param("id"),
			Summary: map[string]any{"execution_id": ex.ID},
		})
		c.JSON(http.StatusCreated, ex)
	}
}

func nextRunsHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		count, _ := strconv.Atoi(c.DefaultQuery("count", "5"))
		times, err := d.Schedules.NextRuns(c.Request.Context(), c.Param("id"), count)
		if err != nil {
			writeScheduleError(c, err)
			return
		}
		ok(c, gin.H{"items": times})
	}
}

type cronPreviewInput struct {
	CronExpr string `json:"cron_expr"`
	Count    int    `json:"count"`
}

func previewCronHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in cronPreviewInput
		if err := c.ShouldBindJSON(&in); err != nil || in.CronExpr == "" {
			fail(c, http.StatusBadRequest, "validation_failed", "请填写 cron 表达式", nil)
			return
		}
		times, err := d.Schedules.Preview(in.CronExpr, in.Count)
		if err != nil {
			writeScheduleError(c, err)
			return
		}
		ok(c, gin.H{"items": times})
	}
}
