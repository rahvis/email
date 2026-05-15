package outbound

import (
	"context"
	"testing"

	"billionmail-core/internal/service/mail_service"
)

type fakeSMTPSender struct {
	messageID  string
	message    mail_service.Message
	recipients []string
	sendErr    error
	closed     bool
}

func (f *fakeSMTPSender) Send(message mail_service.Message, recipients []string) error {
	f.message = message
	f.recipients = recipients
	return f.sendErr
}

func (f *fakeSMTPSender) GenerateMessageID() string {
	return f.messageID
}

func (f *fakeSMTPSender) Close() {
	f.closed = true
}

func TestPostfixSMTPMailerPreservesMessageID(t *testing.T) {
	fake := &fakeSMTPSender{messageID: "<generated@example.com>"}
	mailer := newPostfixSMTPMailerForTesting(fake, "sender@example.com")

	result, err := mailer.Send(context.Background(), OutboundMessage{
		FromEmail: "sender@example.com",
		FromName:  "Sender",
		Recipient: "user@example.com",
		Subject:   "Subject",
		HTML:      "<p>Hello</p>",
		MessageID: "<preserved@example.com>",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if result.Engine != EnginePostfix {
		t.Fatalf("result.Engine = %q, want %q", result.Engine, EnginePostfix)
	}
	if result.MessageID != "<preserved@example.com>" {
		t.Fatalf("result.MessageID = %q", result.MessageID)
	}
	if got := fake.message.MessageID(); got != "<preserved@example.com>" {
		t.Fatalf("message Message-ID = %q", got)
	}
	if len(fake.recipients) != 1 || fake.recipients[0] != "user@example.com" {
		t.Fatalf("recipients = %#v", fake.recipients)
	}
}
