package v115open

import (
	"Q115-STRM/internal/helpers"
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"resty.dev/v3"
)

func TestFileDetailResponseDoesNotLeakIntoLogs(t *testing.T) {
	const secret = "PRIVATE-PICKCODE-AND-SIGNED-URL"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"state":true,"code":0,"message":"` + secret + `","data":{"pick_code":"` + secret + `"}}`))
	}))
	defer server.Close()
	var logs bytes.Buffer
	previous := helpers.V115Log
	helpers.V115Log = &helpers.QLogger{Logger: log.New(&logs, "", 0)}
	defer func() { helpers.V115Log = previous }()
	req := resty.New().R().SetMethod("POST")
	_, response, _, err := (&QueueExecutor{}).executeRequest(&QueuedRequest{URL: server.URL, Request: req})
	if err != nil || response == nil || !strings.Contains(string(response.Data), secret) {
		t.Fatalf("mock response not returned: %v", err)
	}
	if strings.Contains(logs.String(), secret) {
		t.Fatal("115 response leaked into logs")
	}
}
