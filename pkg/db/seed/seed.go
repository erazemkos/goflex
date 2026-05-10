package seed

import "context"

type Func func(context.Context) error

func Seed(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

func All(ctx context.Context, fns ...Func) error {
	for _, fn := range fns {
		if err := fn(ctx); err != nil {
			return err
		}
	}
	return nil
}
