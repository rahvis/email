package outbound

import (
	"context"
	"fmt"
	"net/http"
)

const (
	EnginePostfix = "postfix"
	EngineKumoMTA = "kumomta"

	WorkflowCampaign = "campaign"
	WorkflowAPI      = "api"
	WorkflowSystem   = "system"

	InjectionStatusQueued = "queued"
	InjectionStatusFailed = "failed"

	ErrClassNone      = "none"
	ErrClassRetryable = "retryable"
	ErrClassPermanent = "permanent"
	ErrClassAuth      = "auth"
)

type OutboundMailer interface {
	Send(ctx context.Context, req OutboundMessage) (*OutboundResult, error)
}

type OutboundMessage struct {
	TenantID          int64
	CampaignID        int64
	TaskID            int64
	RecipientID       int64
	APILogID          int64
	APIID             int64
	FromEmail         string
	FromName          string
	Recipient         string
	Subject           string
	HTML              string
	RFC822            []byte
	MessageID         string
	SenderDomain      string
	DestinationDomain string
	SendingProfileID  int64
	Metadata          map[string]string
}

type OutboundResult struct {
	Engine          string
	MessageID       string
	InjectionStatus string
	ProviderQueueID string
	QueueName       string
	AcceptedAt      int64
	RawResponse     string
}

type SendError struct {
	Class      string
	StatusCode int
	Message    string
	Err        error
}

func (e *SendError) Error() string {
	if e == nil {
		return ""
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("%s: HTTP %d: %s", e.Class, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Class, e.Message)
}

func (e *SendError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *SendError) Retryable() bool {
	return e != nil && e.Class == ErrClassRetryable
}

func (e *SendError) Permanent() bool {
	return e != nil && e.Class == ErrClassPermanent
}

func (e *SendError) AuthFailure() bool {
	return e != nil && e.Class == ErrClassAuth
}

func ClassifyHTTPStatus(statusCode int) string {
	switch {
	case statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices:
		return ErrClassNone
	case statusCode == http.StatusBadRequest || statusCode == http.StatusUnprocessableEntity:
		return ErrClassPermanent
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return ErrClassAuth
	case statusCode == http.StatusTooManyRequests:
		return ErrClassRetryable
	case statusCode >= http.StatusInternalServerError:
		return ErrClassRetryable
	default:
		return ErrClassPermanent
	}
}
