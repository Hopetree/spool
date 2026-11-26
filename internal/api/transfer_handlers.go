package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Hopetree/spool/internal/audit"
	"github.com/Hopetree/spool/internal/store"
	"github.com/Hopetree/spool/internal/transfer"
)

func writeTransferError(c *gin.Context, err error) {
	var ve *transfer.ValidationError
	switch {
	case errors.As(err, &ve):
		fail(c, http.StatusBadRequest, "validation_failed", ve.Message, nil)
	case errors.Is(err, transfer.ErrNotFound):
		fail(c, http.StatusNotFound, "not_found", "传输记录不存在", nil)
	default:
		fail(c, http.StatusBadGateway, "transfer_failed", err.Error(), nil)
	}
}

func uploadTransferHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		maxBytes := int64(d.Cfg.Transfer.MaxUploadMB) * 1024 * 1024
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes+1024*1024)
		if err := c.Request.ParseMultipartForm(16 << 20); err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "文件过大或请求格式不正确", nil)
			return
		}
		files := c.Request.MultipartForm.File["file"]
		if len(files) == 0 {
			fail(c, http.StatusBadRequest, "validation_failed", "请选择要上传的文件", nil)
			return
		}
		header := files[0]
		hostIDs := []string{}
		_ = json.Unmarshal([]byte(c.PostForm("host_ids")), &hostIDs)
		remoteDir := c.PostForm("remote_dir")
		if remoteDir == "" {
			fail(c, http.StatusBadRequest, "validation_failed", "请填写远端目录", nil)
			return
		}

		src, err := header.Open()
		if err != nil {
			fail(c, http.StatusBadRequest, "validation_failed", "读取上传文件失败", nil)
			return
		}
		defer src.Close()
		tmpDir := filepath.Join(d.Cfg.DataDir, "tmp")
		if err := os.MkdirAll(tmpDir, 0o755); err != nil {
			fail(c, http.StatusInternalServerError, "internal_error", "创建临时目录失败", nil)
			return
		}
		tmpPath := filepath.Join(tmpDir, "transfer-"+store.NewULID()+".bin")
		dst, err := os.Create(tmpPath)
		if err != nil {
			fail(c, http.StatusInternalServerError, "internal_error", "创建临时文件失败", nil)
			return
		}
		written, copyErr := io.Copy(dst, src)
		_ = dst.Close()
		if copyErr != nil {
			_ = os.Remove(tmpPath)
			fail(c, http.StatusBadRequest, "validation_failed", "文件过大或读取失败", nil)
			return
		}
		if written > maxBytes {
			_ = os.Remove(tmpPath)
			fail(c, http.StatusBadRequest, "validation_failed", fmt.Sprintf("文件超过上限（%d MB）", d.Cfg.Transfer.MaxUploadMB), nil)
			return
		}
		u := currentUser(c)
		createParents := c.PostForm("create_parents") != "false"
		tr, err := d.Transfers.CreateUpload(c.Request.Context(), transfer.UploadRequest{
			FilePath: tmpPath, FileName: header.Filename, Size: written, HostIDs: hostIDs,
			RemoteDir: remoteDir, RemoteName: c.PostForm("remote_name"), Mode: c.PostForm("mode"),
			Overwrite: c.PostForm("overwrite"), CreateParents: createParents, ActorID: u.ID,
		})
		if err != nil {
			_ = os.Remove(tmpPath)
			writeTransferError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "transfer.upload",
			ResourceType: "transfer", ResourceID: tr.ID, ResourceName: tr.FileName,
			Summary: map[string]any{"hosts": tr.TargetTotal, "remote_dir": remoteDir, "size": tr.SizeBytes},
		})
		c.JSON(http.StatusCreated, tr)
	}
}

func listTransfersHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
		items, total, err := d.Transfers.List(c.Request.Context(), page, pageSize)
		if err != nil {
			writeTransferError(c, err)
			return
		}
		ok(c, pageResponse{Items: items, Total: total, Page: page, PageSize: pageSize})
	}
}

func getTransferHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		tr, targets, err := d.Transfers.Get(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeTransferError(c, err)
			return
		}
		ok(c, gin.H{"transfer": tr, "targets": targets})
	}
}

func cancelTransferHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		if err := d.Transfers.Cancel(c.Request.Context(), c.Param("id")); err != nil {
			writeTransferError(c, err)
			return
		}
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "transfer.cancel",
			ResourceType: "transfer", ResourceID: c.Param("id"),
		})
		c.Status(http.StatusNoContent)
	}
}

func downloadTransferHandler(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		hostID := c.Query("host_id")
		remotePath := c.Query("path")
		if hostID == "" || remotePath == "" {
			fail(c, http.StatusBadRequest, "validation_failed", "请提供主机和远端路径", nil)
			return
		}
		u := currentUser(c)
		dl, err := d.Transfers.OpenDownload(c.Request.Context(), hostID, remotePath, u.ID)
		if err != nil {
			writeTransferError(c, err)
			return
		}
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, dl.FileName))
		c.Header("Content-Type", "application/octet-stream")
		c.Header("Content-Length", strconv.FormatInt(dl.Size, 10))
		c.Status(http.StatusOK)
		_, copyErr := io.Copy(c.Writer, dl.Reader)
		dl.Close(copyErr)
		writeAudit(c, d, audit.Entry{
			ActorID: u.ID, ActorName: u.Username, Action: "transfer.download",
			ResourceType: "transfer", ResourceID: dl.TransferID, ResourceName: dl.FileName,
			Summary: map[string]any{"host_id": hostID, "path": remotePath, "size": dl.Size, "ok": copyErr == nil},
		})
	}
}
