package v115open

import (
	"Q115-STRM/internal/helpers"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"testing"

	"resty.dev/v3"
)

type refreshTransport func(*http.Request) (*http.Response, error)

func (f refreshTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRefreshFailureKeepsOrClearsCredentials(t *testing.T) {
	oldApp, old115 := helpers.AppLogger, helpers.V115Log
	helpers.AppLogger = &helpers.QLogger{Logger: log.New(io.Discard, "", 0)}
	helpers.V115Log = &helpers.QLogger{Logger: log.New(io.Discard, "", 0)}
	defer func() { helpers.AppLogger, helpers.V115Log = oldApp, old115 }()

	for _, tc := range []struct {
		name, body            string
		networkError, invalid bool
	}{
		{name: "DNS 失败保留凭据", networkError: true},
		{name: "服务端临时错误保留凭据", body: `{"state":false,"code":500,"message":"temporary"}`},
		{name: "刷新凭证明确失效才清除", body: `{"state":false,"code":40140116,"message":"invalid"}`, invalid: true},
		{name: "成功刷新更新内存凭据", body: `{"state":true,"code":0,"data":{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helpers.InitEventBus()
			invalidEvents := 0
			helpers.SubscribeSync(helpers.V115TokenInValidEvent, func(_ helpers.Event) helpers.EventResult {
				invalidEvents++
				return helpers.EventResult{Success: true}
			})
			transport := refreshTransport(func(r *http.Request) (*http.Response, error) {
				if tc.networkError {
					return nil, errors.New("DNS unavailable")
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body)), Request: r}, nil
			})
			client := &OpenClient{AccountId: 1, client: resty.New().SetTransport(transport), AccessToken: "old-access", RefreshTokenStr: "old-refresh"}
			result, err := client.RefreshToken("")
			if tc.invalid {
				if !errors.Is(err, ErrRefreshTokenInvalid) || invalidEvents != 1 || client.AccessToken != "" || client.RefreshTokenStr != "" {
					t.Fatalf("invalid credentials not cleared exactly once: %v", err)
				}
			} else if tc.networkError || strings.Contains(tc.body, "temporary") {
				if err == nil || invalidEvents != 0 || client.AccessToken != "old-access" || client.RefreshTokenStr != "old-refresh" {
					t.Fatalf("transient error changed credentials: %v", err)
				}
			} else if err != nil || result == nil || client.AccessToken != "new-access" || client.RefreshTokenStr != "new-refresh" || invalidEvents != 0 {
				t.Fatalf("successful refresh failed: %v", err)
			}
		})
	}
}
