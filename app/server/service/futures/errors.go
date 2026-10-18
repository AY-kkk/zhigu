package futures

import "errors"

var (
	ErrNotFound            = errors.New("futures: not found")
	ErrRevisionConflict    = errors.New("futures: revision conflict")
	ErrIdempotencyConflict = errors.New("futures: idempotency conflict")
	ErrInvalidInput        = errors.New("futures: invalid input")
	ErrInvalidCursor       = errors.New("futures: invalid cursor")
	ErrInvalidBatch        = errors.New("futures: invalid notification batch")
	ErrReleaseGate         = errors.New("futures: release gate invalid")
	ErrUnavailable         = errors.New("futures: unavailable")
	ErrForbidden           = errors.New("futures: forbidden")
	ErrReferenced          = errors.New("futures: referenced")
	ErrDocumentLimit       = errors.New("futures: document limit")
	ErrDocumentUnreadable  = errors.New("futures: document unreadable")
	ErrDocumentTimeout     = errors.New("futures: document extraction timeout")
	ErrReadOnly            = errors.New("futures: read only")
	ErrModuleOff           = errors.New("futures: module off")
)
