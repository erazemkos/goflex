package api

import (
	"context"
	"strings"
)

type Context interface{ context.Context }

type Endpoint[Req, Res any] struct {
	Method      string
	Path        string
	Description string
	Handler     func(ctx Context, req Req) (Res, error)
}

func (e *Endpoint[Req, Res]) Register(h func(ctx Context, req Req) (Res, error)) {
	e.Handler = h
	Registry.Register(strings.ToUpper(e.Method), e.Path, e)
}
