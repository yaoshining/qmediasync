package controllers

import (
	"Q115-STRM/internal/models"
	"Q115-STRM/internal/v115open"
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type peachResolverConfig struct {
	token     string
	accountID uint
	root      string
}

type peachLookup func(context.Context, uint, string) (*v115open.FileDetail, error)

// PeachResolve115 为单个固定账号和目录提供最小权限的文件详情查询。
func PeachResolve115(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	id, err := strconv.ParseUint(os.Getenv("PEACH_115_ACCOUNT_ID"), 10, 32)
	if err != nil {
		peachResolveError(c, "查询服务未启用")
		return
	}
	cfg := peachResolverConfig{token: os.Getenv("PEACH_115_TOKEN"), accountID: uint(id), root: os.Getenv("PEACH_115_ROOT")}
	peachResolveHandler(cfg, func(ctx context.Context, id uint, p string) (*v115open.FileDetail, error) {
		account, err := models.GetAccountById(id)
		if err != nil || account.SourceType != models.SourceType115 {
			return nil, errPeachUnavailable{}
		}
		return account.Get115Client().GetFsDetailByPath(ctx, p)
	})(c)
}

type errPeachUnavailable struct{}

func (errPeachUnavailable) Error() string { return "unavailable" }

func peachResolveError(c *gin.Context, message string) {
	c.AbortWithStatusJSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: message, Data: nil})
}

func peachResolveHandler(cfg peachResolverConfig, lookup peachLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("Referrer-Policy", "no-referrer")
		if len(cfg.token) < 32 || cfg.accountID == 0 || !validPeachRoot(cfg.root) {
			peachResolveError(c, "查询服务未启用")
			return
		}
		auth := c.GetHeader("Authorization")
		expected := "Bearer " + cfg.token
		if subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) != 1 {
			peachResolveError(c, "未授权")
			return
		}
		if c.Request.URL.RawQuery != "" {
			peachResolveError(c, "参数错误")
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
		var req struct {
			Path string `json:"path"`
			Size int64  `json:"size"`
		}
		decoder := json.NewDecoder(c.Request.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&req) != nil || req.Size <= 0 || !validPeachRelativePath(req.Path) {
			peachResolveError(c, "参数错误")
			return
		}
		var trailing any
		if decoder.Decode(&trailing) != io.EOF {
			peachResolveError(c, "参数错误")
			return
		}
		full := path.Join(cfg.root, req.Path)
		detail, err := lookup(c.Request.Context(), cfg.accountID, full)
		if err != nil || detail == nil {
			peachResolveError(c, "文件查询失败")
			return
		}
		if detail.FileCategory != v115open.TypeFile || detail.FileId == "" || detail.PickCode == "" ||
			detail.FileSizeByte != req.Size || path.Join("/", detail.Path, detail.FileName) != full {
			peachResolveError(c, "文件信息不一致")
			return
		}
		c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "查询成功", Data: gin.H{"file_id": detail.FileId, "pick_code": detail.PickCode, "size": detail.FileSizeByte}})
	}
}

func validPeachRoot(root string) bool {
	return root != "/" && strings.HasPrefix(root, "/") && path.Clean(root) == root && !strings.ContainsAny(root, "\\%\x00")
}

func validPeachRelativePath(p string) bool {
	if p == "" || len(p) > 3072 || strings.HasPrefix(p, "/") || strings.ContainsAny(p, "\\%\x00") {
		return false
	}
	if path.Clean(p) != p {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if part == "." || part == ".." || part == "" {
			return false
		}
	}
	return true
}
