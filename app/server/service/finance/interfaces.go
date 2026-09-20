package finance

import (
	"context"
	"fmt"
)

type ResearchClient interface {
	Submit(ctx context.Context, task ResearchTask) (TaskReceipt, error)
	Get(ctx context.Context, taskID string) (TaskSnapshot, error)
	Cancel(ctx context.Context, taskID string) (TaskSnapshot, error)
	Purge(ctx context.Context, taskID string) error
}

type EvidenceGate interface {
	ValidateRole(ctx context.Context, run RunSnapshot, result ResearchResult) (ValidatedRole, error)
}

type BudgetService interface {
	Reserve(ctx context.Context, request BudgetRequest) (Reservation, error)
	Reconcile(ctx context.Context, reservationID string, usage ObservedUsage) error
}

type ReportPublisher interface {
	Publish(ctx context.Context, runID string, expectedVersion int64, report VerifiedReport) error
}

type TaskReceipt struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
}

type TaskSnapshot struct {
	TaskID string          `json:"task_id"`
	Status string          `json:"status"`
	Result *ResearchResult `json:"result"`
}

type BudgetRequest struct {
	RequestID      string
	OwnerID        uint
	RunID          string
	TaskID         string
	ParseID        string
	QuestionID     string
	Purpose        string
	Kind           string
	BodyHash       string
	ReservedInput  int
	ReservedOutput int
}

type Reservation struct {
	ID             string
	RequestID      string
	Status         string
	ReservedInput  int
	ReservedOutput int
}

type ObservedUsage struct {
	ActualInput       *int
	ActualOutput      *int
	Unknown           bool
	UpstreamRequestID string
}

type AppError struct {
	Class   string
	Code    string
	Message string
	Retry   bool
	Status  int
}

func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func NewError(status int, class, code, message string) *AppError {
	return &AppError{Status: status, Class: class, Code: code, Message: message}
}
