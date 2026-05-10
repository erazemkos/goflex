package sessionstore

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

type gormSession struct {
	ID         string `gorm:"primaryKey"`
	UserID     string
	ExpiresAt  time.Time
	ValuesJSON string
}

type GORM struct{ DB *gorm.DB }

func NewGORM(db *gorm.DB) *GORM { _ = db.AutoMigrate(&gormSession{}); return &GORM{DB: db} }
func (s *GORM) Get(ctx context.Context, id string) (Session, error) {
	var row gormSession
	err := s.DB.WithContext(ctx).First(&row, "id = ?", id).Error
	if err != nil {
		return Session{}, err
	}
	if time.Now().After(row.ExpiresAt) {
		_ = s.Delete(ctx, id)
		return Session{}, gorm.ErrRecordNotFound
	}
	sess := Session{ID: row.ID, UserID: row.UserID, ExpiresAt: row.ExpiresAt}
	_ = json.Unmarshal([]byte(row.ValuesJSON), &sess.Values)
	return sess, nil
}
func (s *GORM) Set(ctx context.Context, sess Session) error {
	b, _ := json.Marshal(sess.Values)
	return s.DB.WithContext(ctx).Save(&gormSession{ID: sess.ID, UserID: sess.UserID, ExpiresAt: sess.ExpiresAt, ValuesJSON: string(b)}).Error
}
func (s *GORM) Delete(ctx context.Context, id string) error {
	return s.DB.WithContext(ctx).Delete(&gormSession{}, "id = ?", id).Error
}
func (s *GORM) Touch(ctx context.Context, id string, exp time.Time) error {
	return s.DB.WithContext(ctx).Model(&gormSession{}).Where("id = ?", id).Update("expires_at", exp).Error
}
func (s *GORM) Cleanup(ctx context.Context) error {
	return s.DB.WithContext(ctx).Delete(&gormSession{}, "expires_at < ?", time.Now()).Error
}
