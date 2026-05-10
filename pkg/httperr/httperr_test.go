package httperr

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNewAndError(t *testing.T) {
	err := New("validation_failed", "bad input", map[string]string{"title": "required"})
	if err.Error() != "bad input" {
		t.Fatalf("Error() = %q", err.Error())
	}
	if (*Error)(nil).Error() != "" {
		t.Fatal("nil Error should stringify to empty string")
	}
}

func TestWriteJSONEnvelopeWithRequestIDAndLogger(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	var logs bytes.Buffer
	rr := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rr)
	c.Request = httptest.NewRequest(http.MethodPost, "/todos", nil)
	c.Request.Header.Set("X-Request-Id", "req-123")
	c.Set(loggerKey, slog.New(slog.NewTextHandler(&logs, nil)))

	Write(c, http.StatusUnprocessableEntity, New("validation_failed", "bad input", map[string]string{"title": "required"}))

	got := strings.TrimSpace(rr.Body.String())
	want := `{"code":"validation_failed","message":"bad input","fields":{"title":"required"},"request_id":"req-123"}`
	if got != want {
		t.Fatalf("body mismatch\nwant %s\n got %s", want, got)
	}
	if !strings.Contains(logs.String(), "req-123") || !strings.Contains(logs.String(), "validation_failed") {
		t.Fatalf("log missing request id/code: %s", logs.String())
	}
}

func TestWriteWrapsPlainErrors(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	rr := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rr)
	c.Request = httptest.NewRequest(http.MethodGet, "/missing", nil)

	Write(c, http.StatusBadGateway, errors.New("upstream unavailable"))

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"code":"bad_gateway"`) || !strings.Contains(rr.Body.String(), "upstream unavailable") {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestWriteNilError(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	rr := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rr)
	c.Request = httptest.NewRequest(http.MethodGet, "/missing", nil)

	Write(c, http.StatusNotFound, nil)

	if rr.Code != http.StatusNotFound || !strings.Contains(rr.Body.String(), `"code":"not_found"`) {
		t.Fatalf("response = %d %s", rr.Code, rr.Body.String())
	}
}
