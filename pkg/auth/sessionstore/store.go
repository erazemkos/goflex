package sessionstore

import (
	"context"
	"time"
)

type Session struct {
	ID, UserID string
	ExpiresAt  time.Time
	Values     map[string]string
}
type Store interface {
	Get(context.Context, string) (Session, error)
	Set(context.Context, Session) error
	Delete(context.Context, string) error
	Touch(context.Context, string, time.Time) error
}
