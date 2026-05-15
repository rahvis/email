package outbound

import (
	"context"
	"fmt"
	"strings"
	"time"

	"billionmail-core/internal/service/mail_service"
)

type smtpSender interface {
	Send(message mail_service.Message, recipients []string) error
	GenerateMessageID() string
	Close()
}

type PostfixSMTPMailer struct {
	sender      smtpSender
	fromEmail   string
	closeOnSend bool
}

func NewPostfixSMTPMailer(addresser string) (*PostfixSMTPMailer, error) {
	sender, err := mail_service.NewEmailSenderWithLocal(addresser)
	if err != nil {
		return nil, err
	}
	return &PostfixSMTPMailer{
		sender:      sender,
		fromEmail:   addresser,
		closeOnSend: true,
	}, nil
}

func NewPostfixSMTPMailerWithSender(sender *mail_service.EmailSender) *PostfixSMTPMailer {
	fromEmail := ""
	if sender != nil {
		fromEmail = sender.Email
	}
	return &PostfixSMTPMailer{
		sender:    sender,
		fromEmail: fromEmail,
	}
}

func newPostfixSMTPMailerForTesting(sender smtpSender, fromEmail string) *PostfixSMTPMailer {
	return &PostfixSMTPMailer{sender: sender, fromEmail: fromEmail}
}

func (m *PostfixSMTPMailer) Send(ctx context.Context, req OutboundMessage) (*OutboundResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if m == nil || m.sender == nil {
		return nil, fmt.Errorf("postfix sender is not configured")
	}
	if strings.TrimSpace(req.Recipient) == "" {
		return nil, fmt.Errorf("recipient is required")
	}
	messageID := strings.TrimSpace(req.MessageID)
	if messageID == "" {
		messageID = m.sender.GenerateMessageID()
	}

	content := req.HTML
	if content == "" && len(req.RFC822) > 0 {
		content = string(req.RFC822)
	}
	message := mail_service.NewMessage(req.Subject, content)
	message.SetMessageID(messageID)
	if req.FromName != "" {
		message.SetRealName(req.FromName)
	}

	if err := m.sender.Send(message, []string{req.Recipient}); err != nil {
		return nil, err
	}

	return &OutboundResult{
		Engine:          EnginePostfix,
		MessageID:       messageID,
		InjectionStatus: InjectionStatusQueued,
		AcceptedAt:      time.Now().Unix(),
	}, nil
}

func (m *PostfixSMTPMailer) GenerateMessageID() string {
	if m == nil || m.sender == nil {
		return ""
	}
	return m.sender.GenerateMessageID()
}

func (m *PostfixSMTPMailer) Close() {
	if m == nil || !m.closeOnSend || m.sender == nil {
		return
	}
	m.sender.Close()
}
