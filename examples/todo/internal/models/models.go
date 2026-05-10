package models

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"github.com/goflex/goflex/examples/todo/shared"
	"gorm.io/gorm"
)

type User struct {
	ID           uint   `gorm:"primaryKey"`
	Email        string `gorm:"uniqueIndex;size:255;not null"`
	Name         string `gorm:"size:255"`
	PasswordHash string `gorm:"not null"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Todo struct {
	ID        uint   `gorm:"primaryKey"`
	UserID    uint   `gorm:"index;not null;uniqueIndex:idx_user_title"`
	Title     string `gorm:"size:120;not null;uniqueIndex:idx_user_title"`
	Done      bool   `gorm:"not null;default:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Store struct {
	db         *gorm.DB
	failToggle atomic.Bool
}

func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

func (s *Store) AutoMigrate() error { return s.db.AutoMigrate(&User{}, &Todo{}) }

func (s *Store) SetToggleFailure(enabled bool) { s.failToggle.Store(enabled) }

func (s *Store) CreateUser(ctx context.Context, email, passwordHash string) (shared.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	u := User{Email: email, Name: email, PasswordHash: passwordHash}
	if err := s.db.WithContext(ctx).Create(&u).Error; err != nil {
		return shared.User{}, shared.ErrConflict
	}
	return userDTO(u), nil
}

func (s *Store) UserByEmail(ctx context.Context, email string) (shared.User, string, error) {
	var u User
	if err := s.db.WithContext(ctx).Where("email = ?", strings.ToLower(strings.TrimSpace(email))).First(&u).Error; err != nil {
		return shared.User{}, "", shared.ErrNotFound
	}
	return userDTO(u), u.PasswordHash, nil
}

func (s *Store) UserByID(ctx context.Context, id uint) (shared.User, error) {
	var u User
	if err := s.db.WithContext(ctx).First(&u, id).Error; err != nil {
		return shared.User{}, shared.ErrNotFound
	}
	return userDTO(u), nil
}

func (s *Store) UpdatePassword(ctx context.Context, userID uint, passwordHash string) (shared.User, error) {
	var u User
	if err := s.db.WithContext(ctx).First(&u, userID).Error; err != nil {
		return shared.User{}, shared.ErrNotFound
	}
	u.PasswordHash = passwordHash
	if err := s.db.WithContext(ctx).Save(&u).Error; err != nil {
		return shared.User{}, err
	}
	return userDTO(u), nil
}

func (s *Store) ListTodos(ctx context.Context, userID uint, filter, q string, limit int) ([]shared.Todo, error) {
	var rows []Todo
	db := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("id asc")
	switch filter {
	case "open":
		db = db.Where("done = ?", false)
	case "done":
		db = db.Where("done = ?", true)
	}
	if q = strings.TrimSpace(q); q != "" {
		db = db.Where("title like ?", "%"+q+"%")
	}
	if limit > 0 {
		db = db.Limit(limit)
	}
	if err := db.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]shared.Todo, 0, len(rows))
	for _, row := range rows {
		out = append(out, todoDTO(row))
	}
	return out, nil
}

func (s *Store) CreateTodo(ctx context.Context, userID uint, title string) (shared.Todo, error) {
	row := Todo{UserID: userID, Title: strings.TrimSpace(title)}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return shared.Todo{}, shared.ErrConflict
	}
	return todoDTO(row), nil
}

func (s *Store) TodoByID(ctx context.Context, userID, id uint) (shared.Todo, error) {
	var row Todo
	if err := s.db.WithContext(ctx).Where("user_id = ? AND id = ?", userID, id).First(&row).Error; err != nil {
		return shared.Todo{}, shared.ErrNotFound
	}
	return todoDTO(row), nil
}

func (s *Store) UpdateTodo(ctx context.Context, userID, id uint, title string, done *bool) (shared.Todo, error) {
	var row Todo
	if err := s.db.WithContext(ctx).Where("user_id = ? AND id = ?", userID, id).First(&row).Error; err != nil {
		return shared.Todo{}, shared.ErrNotFound
	}
	if done != nil && *done != row.Done && s.failToggle.Load() {
		return shared.Todo{}, shared.StoreError("toggle failed")
	}
	if strings.TrimSpace(title) != "" {
		row.Title = strings.TrimSpace(title)
	}
	if done != nil {
		row.Done = *done
	}
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return shared.Todo{}, shared.ErrConflict
	}
	return todoDTO(row), nil
}

func (s *Store) DeleteTodo(ctx context.Context, userID, id uint) error {
	res := s.db.WithContext(ctx).Where("user_id = ? AND id = ?", userID, id).Delete(&Todo{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func userDTO(u User) shared.User { return shared.User{ID: u.ID, Email: u.Email, Name: u.Name} }
func todoDTO(t Todo) shared.Todo {
	return shared.Todo{ID: t.ID, Title: t.Title, Done: t.Done, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
}
