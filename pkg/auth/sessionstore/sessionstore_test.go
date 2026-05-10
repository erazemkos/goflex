package sessionstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMemoryStoreCRUDAndExpiration(t *testing.T) {
	store := NewMemory()
	ctx := context.Background()
	sess := Session{ID: "s1", UserID: "u1", ExpiresAt: time.Now().Add(time.Hour), Values: map[string]string{"role": "admin"}}
	if err := store.Set(ctx, sess); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, "s1")
	if err != nil || got.UserID != "u1" || got.Values["role"] != "admin" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	newExp := time.Now().Add(2 * time.Hour)
	if err := store.Touch(ctx, "s1", newExp); err != nil {
		t.Fatal(err)
	}
	got, err = store.Get(ctx, "s1")
	if err != nil || !got.ExpiresAt.Equal(newExp) {
		t.Fatalf("touch got=%+v err=%v", got, err)
	}
	if err := store.Delete(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "s1"); err == nil {
		t.Fatal("expected deleted session error")
	}

	if err := store.Set(ctx, Session{ID: "expired", UserID: "u1", ExpiresAt: time.Now().Add(-time.Second)}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "expired"); err == nil {
		t.Fatal("expected expired session error")
	}
}

func TestCookieStoreSignedRoundTripAndTamper(t *testing.T) {
	store := NewCookie([]byte("secret"))
	sess := Session{ID: "s1", UserID: "u1", ExpiresAt: time.Now().Add(time.Hour), Values: map[string]string{"x": "y"}}
	encoded, err := store.Encode(sess)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), encoded)
	if err != nil || got.ID != "s1" || got.UserID != "u1" || got.Values["x"] != "y" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if _, err := store.Get(context.Background(), encoded+"x"); err == nil {
		t.Fatal("tampered cookie should fail")
	}
	expired, err := store.Encode(Session{ID: "old", UserID: "u1", ExpiresAt: time.Now().Add(-time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), expired); err == nil {
		t.Fatal("expired cookie should fail")
	}
	if err := store.Set(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	if err := store.Touch(context.Background(), "unused", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), "unused"); err != nil {
		t.Fatal(err)
	}
}

func TestGORMStoreCRUDAndCleanup(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	store := NewGORM(db)
	ctx := context.Background()
	exp := time.Now().Add(time.Hour).Truncate(time.Millisecond)
	sess := Session{ID: "s1", UserID: "u1", ExpiresAt: exp, Values: map[string]string{"role": "admin"}}
	if err := store.Set(ctx, sess); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, "s1")
	if err != nil || got.ID != "s1" || got.UserID != "u1" || got.Values["role"] != "admin" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	newExp := time.Now().Add(2 * time.Hour).Truncate(time.Millisecond)
	if err := store.Touch(ctx, "s1", newExp); err != nil {
		t.Fatal(err)
	}
	got, err = store.Get(ctx, "s1")
	if err != nil || got.ExpiresAt.Before(newExp.Add(-time.Second)) {
		t.Fatalf("touch got=%+v err=%v", got, err)
	}
	if err := store.Delete(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "s1"); err == nil {
		t.Fatal("deleted session should fail")
	}

	if err := store.Set(ctx, Session{ID: "old", UserID: "u1", ExpiresAt: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "old"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expired get err=%v", err)
	}
	if err := store.Set(ctx, Session{ID: "old2", UserID: "u1", ExpiresAt: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := store.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "old2"); err == nil {
		t.Fatal("cleanup should remove expired rows")
	}
}
