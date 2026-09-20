package finance

import "context"

type ctxKey string

const (
	ctxUserID    ctxKey = "user_id"
	ctxRole      ctxKey = "role"
	ctxTaskToken ctxKey = "task_token"
)

func WithUser(ctx context.Context, userID uint, role string) context.Context {
	ctx = context.WithValue(ctx, ctxUserID, userID)
	ctx = context.WithValue(ctx, ctxRole, role)
	return ctx
}

func WithTaskToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, ctxTaskToken, token)
}

func TaskTokenFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxTaskToken).(string)
	return v
}

func UserIDFrom(ctx context.Context) uint {
	v, _ := ctx.Value(ctxUserID).(uint)
	return v
}

func RoleFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxRole).(string)
	return v
}
