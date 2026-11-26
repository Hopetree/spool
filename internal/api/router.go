package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Hopetree/spool/internal/audit"
	"github.com/Hopetree/spool/internal/auth"
	"github.com/Hopetree/spool/internal/config"
	"github.com/Hopetree/spool/internal/credential"
	"github.com/Hopetree/spool/internal/exec"
	"github.com/Hopetree/spool/internal/flow"
	"github.com/Hopetree/spool/internal/host"
	"github.com/Hopetree/spool/internal/store"
	"github.com/Hopetree/spool/internal/tool"
)

// Deps carries the services the HTTP layer needs.
type Deps struct {
	Cfg         *config.Config
	Store       *store.Store
	Auth        *auth.Service
	Audit       *audit.Service
	Credentials *credential.Service
	Hosts       *host.Service
	Tools       *tool.Service
	Flows       *flow.Service
	Engine      *exec.Engine
	Version     string
	StartedAt   time.Time
	WebHandler  http.Handler
}

// NewRouter builds the API router and SPA fallback.
func NewRouter(d Deps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), SecurityHeaders(), OriginCheck(d.Cfg), requestLog())

	v1 := r.Group("/api/v1")
	v1.GET("/version", func(c *gin.Context) {
		ok(c, gin.H{
			"version":    d.Version,
			"started_at": d.StartedAt.UTC().Format(time.RFC3339),
		})
	})

	limiter := newLoginLimiter(d.Cfg.Security.LoginMaxFailures,
		time.Duration(d.Cfg.Security.LoginLockoutMinutes)*time.Minute, time.Now)
	v1.POST("/auth/login", loginHandler(d, limiter))

	authed := v1.Group("")
	authed.Use(RequireAuth(d.Auth), ForcePasswordChange())
	authed.GET("/auth/me", meHandler())
	authed.POST("/auth/logout", logoutHandler(d))
	authed.PUT("/auth/password", changePasswordHandler(d))

	admin := authed.Group("")
	admin.Use(RequireRole("admin"))
	admin.GET("/credentials", listCredentialsHandler(d))
	admin.POST("/credentials", createCredentialHandler(d))
	admin.PUT("/credentials/:id", updateCredentialHandler(d))
	admin.DELETE("/credentials/:id", deleteCredentialHandler(d))
	admin.POST("/hosts", createHostHandler(d))
	admin.PUT("/hosts/:id", updateHostHandler(d))
	admin.DELETE("/hosts/:id", deleteHostHandler(d))
	admin.PUT("/hosts/:id/fingerprint", setFingerprintHandler(d))
	admin.POST("/host-groups", createGroupHandler(d))
	admin.PUT("/host-groups/:id", updateGroupHandler(d))
	admin.DELETE("/host-groups/:id", deleteGroupHandler(d))
	admin.POST("/tools", createToolHandler(d))
	admin.PUT("/tools/:id", updateToolHandler(d))
	admin.DELETE("/tools/:id", deleteToolHandler(d))
	admin.POST("/tools/:id/enabled", setToolEnabledHandler(d))
	admin.POST("/tools/:id/rollback", rollbackToolHandler(d))
	admin.POST("/tools/:id/copy", copyToolHandler(d))
	admin.POST("/flows", createFlowHandler(d))
	admin.PUT("/flows/:id", updateFlowHandler(d))
	admin.DELETE("/flows/:id", deleteFlowHandler(d))
	admin.POST("/flows/:id/enabled", setFlowEnabledHandler(d))
	admin.POST("/flows/:id/copy", copyFlowHandler(d))

	authed.GET("/hosts", listHostsHandler(d))
	authed.GET("/hosts/:id", getHostHandler(d))
	authed.GET("/host-groups", listGroupsHandler(d))
	authed.GET("/tools", listToolsHandler(d))
	authed.GET("/tools/:id", getToolHandler(d))
	authed.GET("/tools/:id/versions", toolVersionsHandler(d))
	authed.GET("/tools/:id/versions/:no", toolVersionHandler(d))
	authed.GET("/flows", listFlowsHandler(d))
	authed.GET("/flows/:id", getFlowHandler(d))
	authed.GET("/executions", listExecutionsHandler(d))
	authed.GET("/executions/:id", getExecutionHandler(d))
	authed.GET("/executions/:id/events", executionEventsHandler(d))
	authed.GET("/executions/:id/stream", streamExecutionHandler(d))
	authed.GET("/executions/:id/logs/download", downloadExecutionLogHandler(d))

	ops := authed.Group("")
	ops.Use(RequireRole("operator"))
	ops.POST("/hosts/:id/test", testHostHandler(d))
	ops.POST("/executions", createExecutionHandler(d))
	ops.POST("/executions/:id/cancel", cancelExecutionHandler(d))
	ops.POST("/executions/:id/rerun", rerunExecutionHandler(d))
	ops.POST("/executions/:id/host-runs/:hostRunId/cancel", cancelHostRunHandler(d))

	if d.WebHandler != nil {
		r.NoRoute(gin.WrapH(d.WebHandler))
	} else {
		r.NoRoute(func(c *gin.Context) {
			fail(c, http.StatusNotFound, "not_found", "资源不存在", nil)
		})
	}
	return r
}
