package httperr

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	requestIDHeader = "X-Request-Id"
	loggerKey       = "goflex.logger"
)

type Error struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
	RequestID string            `json:"request_id,omitempty"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

func New(code, message string, fields map[string]string) *Error {
	return &Error{Code: code, Message: message, Fields: fields}
}

func Write(c *gin.Context, status int, err error) {
	envelope := envelope(status, err)
	if rid := requestID(c); rid != "" {
		envelope.RequestID = rid
	}
	logger(c).Error("http error", "status", status, "code", envelope.Code, "message", envelope.Message, "request_id", envelope.RequestID)
	c.AbortWithStatusJSON(status, envelope)
}

func envelope(status int, err error) Error {
	if err == nil {
		return Error{Code: statusCode(status), Message: http.StatusText(status)}
	}
	var he *Error
	if errors.As(err, &he) && he != nil {
		return Error{Code: he.Code, Message: he.Message, Fields: he.Fields, RequestID: he.RequestID}
	}
	return Error{Code: statusCode(status), Message: err.Error()}
}

func requestID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if rid := c.GetHeader(requestIDHeader); rid != "" {
		return rid
	}
	return c.Writer.Header().Get(requestIDHeader)
}

func logger(c *gin.Context) *slog.Logger {
	if c != nil {
		if v, ok := c.Get(loggerKey); ok {
			if logger, ok := v.(*slog.Logger); ok && logger != nil {
				return logger
			}
		}
	}
	return slog.Default()
}

func statusCode(status int) string {
	text := strings.ToLower(http.StatusText(status))
	text = strings.ReplaceAll(text, " ", "_")
	if text == "" {
		return "error"
	}
	return text
}
