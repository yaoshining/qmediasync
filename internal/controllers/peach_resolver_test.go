package controllers

import (
	"Q115-STRM/internal/v115open"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPeachResolverAuthorizationAndFileValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const token = "0123456789abcdef0123456789abcdef"
	cfg := peachResolverConfig{token: token, accountID: 7, root: "/JAV"}
	tests := []struct {
		name, auth, body, query string
		change                  func(*v115open.FileDetail)
		want                    int
		called                  bool
	}{
		{name: "成功", auth: token, body: `{"path":"folder/movie.mp4","size":123}`, want: 200, called: true},
		{name: "缺少令牌", body: `{"path":"folder/movie.mp4","size":123}`, want: 401},
		{name: "错误令牌", auth: token + "x", body: `{"path":"folder/movie.mp4","size":123}`, want: 401},
		{name: "路径越界", auth: token, body: `{"path":"../outside.mp4","size":123}`, want: 400},
		{name: "编码路径", auth: token, body: `{"path":"folder/%2e%2e/a","size":123}`, want: 400},
		{name: "绝对路径", auth: token, body: `{"path":"/JAV/movie.mp4","size":123}`, want: 400},
		{name: "额外字段", auth: token, body: `{"path":"folder/movie.mp4","size":123,"account_id":8}`, want: 400},
		{name: "查询参数", auth: token, query: "?api_key=bad", body: `{"path":"folder/movie.mp4","size":123}`, want: 400},
		{name: "多个对象", auth: token, body: `{"path":"folder/movie.mp4","size":123}{}`, want: 400},
		{name: "大小不符", auth: token, body: `{"path":"folder/movie.mp4","size":123}`, change: func(d *v115open.FileDetail) { d.FileSizeByte = 124 }, want: 409, called: true},
		{name: "路径不符", auth: token, body: `{"path":"folder/movie.mp4","size":123}`, change: func(d *v115open.FileDetail) { d.Path = "JAV/other" }, want: 409, called: true},
		{name: "缺少提取码", auth: token, body: `{"path":"folder/movie.mp4","size":123}`, change: func(d *v115open.FileDetail) { d.PickCode = "" }, want: 409, called: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			lookup := func(_ context.Context, id uint, p string) (*v115open.FileDetail, error) {
				called = true
				if id != 7 || p != "/JAV/folder/movie.mp4" {
					return nil, errors.New("wrong scope")
				}
				d := &v115open.FileDetail{FileId: "id", FileName: "movie.mp4", FileSizeByte: 123, Path: "JAV/folder", FileCategory: v115open.TypeFile, PickCode: "private"}
				if tt.change != nil {
					tt.change(d)
				}
				return d, nil
			}
			r := gin.New()
			r.POST("/resolve", peachResolveHandler(cfg, lookup))
			req := httptest.NewRequest(http.MethodPost, "/resolve"+tt.query, strings.NewReader(tt.body))
			if tt.auth != "" {
				req.Header.Set("Authorization", "Bearer "+tt.auth)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			var body struct {
				Code int            `json:"code"`
				Data map[string]any `json:"data"`
			}
			if json.Unmarshal(w.Body.Bytes(), &body) != nil || w.Code != http.StatusOK || called != tt.called {
				t.Fatalf("status=%d called=%v", w.Code, called)
			}
			if (tt.want == 200 && body.Code != 200) || (tt.want != 200 && body.Code != 500) {
				t.Fatalf("code=%d", body.Code)
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("missing no-store")
			}
			if tt.want == 200 {
				if body.Data["pick_code"] != "private" {
					t.Fatal("missing result")
				}
			}
		})
	}
}

func TestPeachResolverDisabledByDefault(t *testing.T) {
	r := gin.New()
	r.POST("/resolve", peachResolveHandler(peachResolverConfig{}, nil))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/resolve", nil))
	var body struct {
		Code int `json:"code"`
	}
	if json.Unmarshal(w.Body.Bytes(), &body) != nil || w.Code != http.StatusOK || body.Code != 500 {
		t.Fatalf("status=%d code=%d", w.Code, body.Code)
	}
}
