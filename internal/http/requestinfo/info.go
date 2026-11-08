package requestinfo

import "context"

type (
	key  struct{}
	Info struct {
		Route     string
		Status    int
		Err       error
		ErrorType string
		Stack     []byte
		Logged    bool
	}
)

func With(ctx context.Context, info *Info) context.Context {
	return context.WithValue(ctx, key{}, info)
}
func From(ctx context.Context) *Info { info, _ := ctx.Value(key{}).(*Info); return info }
