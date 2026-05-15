package outbound

import (
	"context"
	"strings"
)

func SelectMailer(ctx context.Context, workflow, requestedEngine, addresser string) (OutboundMailer, string, error) {
	engine := strings.ToLower(strings.TrimSpace(requestedEngine))
	switch engine {
	case EngineKumoMTA, "kumo":
		mailer, err := NewKumoHTTPMailer(ctx)
		return mailer, EngineKumoMTA, err
	case "", EnginePostfix, "local", "smtp":
		mailer, err := NewPostfixSMTPMailer(addresser)
		return mailer, EnginePostfix, err
	default:
		mailer, err := NewPostfixSMTPMailer(addresser)
		return mailer, EnginePostfix, err
	}
}
