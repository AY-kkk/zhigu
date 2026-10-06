package futures

import (
	"context"

	model "zhigu/server/model/futures"
)

type Identity struct {
	OwnerID uint
	Mode    string
	Role    string
}

type Repository interface {
	CreateDraft(context.Context, Identity, *model.Draft) error
	GetDraft(context.Context, Identity, string) (*model.Draft, error)
	DeleteDraft(context.Context, Identity, string, int64) error
	CreateRun(context.Context, Identity, *model.Run) error
	GetRun(context.Context, Identity, string) (*model.Run, error)
	DeleteRun(context.Context, Identity, string) error
}
