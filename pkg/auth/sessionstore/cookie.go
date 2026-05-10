package sessionstore

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type Cookie struct{ Secret []byte }

func NewCookie(secret []byte) *Cookie { return &Cookie{Secret: secret} }
func (s *Cookie) Encode(sess Session) (string, error) {
	b, err := json.Marshal(sess)
	if err != nil {
		return "", err
	}
	sig := sign(s.Secret, b)
	return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}
func (s *Cookie) Decode(v string) (Session, error) {
	parts := strings.Split(v, ".")
	if len(parts) != 2 {
		return Session{}, errors.New("bad cookie")
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Session{}, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Session{}, err
	}
	if !hmac.Equal(sig, sign(s.Secret, b)) {
		return Session{}, errors.New("bad signature")
	}
	var sess Session
	err = json.Unmarshal(b, &sess)
	return sess, err
}
func sign(secret, b []byte) []byte { h := hmac.New(sha256.New, secret); h.Write(b); return h.Sum(nil) }
func (s *Cookie) Get(ctx context.Context, id string) (Session, error) {
	sess, err := s.Decode(id)
	if err != nil || time.Now().After(sess.ExpiresAt) {
		return Session{}, errors.New("session not found")
	}
	return sess, nil
}
func (s *Cookie) Set(ctx context.Context, sess Session) error               { return nil }
func (s *Cookie) Delete(ctx context.Context, id string) error               { return nil }
func (s *Cookie) Touch(ctx context.Context, id string, exp time.Time) error { return nil }
