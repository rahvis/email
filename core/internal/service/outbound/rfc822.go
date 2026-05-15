package outbound

import (
	"bytes"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"time"
)

func BuildRFC822(req OutboundMessage) ([]byte, error) {
	if len(req.RFC822) > 0 {
		return ensureRFC822Headers(req.RFC822, CorrelationHeaders(req)), nil
	}
	if strings.TrimSpace(req.FromEmail) == "" {
		return nil, fmt.Errorf("from email is required")
	}
	if strings.TrimSpace(req.Recipient) == "" {
		return nil, fmt.Errorf("recipient is required")
	}

	headers := map[string]string{
		"From":                      formatAddress(req.FromName, req.FromEmail),
		"To":                        req.Recipient,
		"Subject":                   mime.QEncoding.Encode("UTF-8", req.Subject),
		"Date":                      time.Now().Format(time.RFC1123Z),
		"MIME-Version":              "1.0",
		"Content-Type":              "text/html; charset=utf-8",
		"Content-Transfer-Encoding": "quoted-printable",
		"X-Mailer":                  "BillionMail",
	}
	if req.MessageID != "" {
		headers["Message-ID"] = req.MessageID
	}
	for key, value := range CorrelationHeaders(req) {
		headers[key] = value
	}

	var buf bytes.Buffer
	headerOrder := []string{
		"From",
		"To",
		"Subject",
		"Date",
		"Message-ID",
		"MIME-Version",
		"Content-Type",
		"Content-Transfer-Encoding",
		"X-Mailer",
		"X-BM-Tenant-ID",
		"X-BM-Campaign-ID",
		"X-BM-Task-ID",
		"X-BM-Recipient-ID",
		"X-BM-Api-ID",
		"X-BM-Api-Log-ID",
		"X-BM-Message-ID",
		"X-BM-Sending-Profile-ID",
		"X-BM-Engine",
	}
	written := map[string]bool{}
	for _, key := range headerOrder {
		value := strings.TrimSpace(headers[key])
		if value == "" {
			continue
		}
		fmt.Fprintf(&buf, "%s: %s\r\n", key, sanitizeHeaderValue(value))
		written[key] = true
	}
	for _, key := range SortedHeaderKeys(headers) {
		if written[key] {
			continue
		}
		value := strings.TrimSpace(headers[key])
		if value == "" {
			continue
		}
		fmt.Fprintf(&buf, "%s: %s\r\n", key, sanitizeHeaderValue(value))
	}
	buf.WriteString("\r\n")

	qp := quotedprintable.NewWriter(&buf)
	if _, err := qp.Write([]byte(req.HTML)); err != nil {
		_ = qp.Close()
		return nil, err
	}
	if err := qp.Close(); err != nil {
		return nil, err
	}
	buf.WriteString("\r\n")
	return buf.Bytes(), nil
}

func ensureRFC822Headers(raw []byte, headers map[string]string) []byte {
	if len(headers) == 0 {
		return raw
	}

	rawMessage := strings.ReplaceAll(string(raw), "\r\n", "\n")
	rawMessage = strings.ReplaceAll(rawMessage, "\r", "\n")
	parts := strings.SplitN(rawMessage, "\n\n", 2)
	headerBlock := parts[0]
	body := ""
	if len(parts) == 2 {
		body = parts[1]
	}

	existing := map[string]bool{}
	for _, line := range strings.Split(headerBlock, "\n") {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		name, _, ok := strings.Cut(line, ":")
		if ok {
			existing[strings.ToLower(strings.TrimSpace(name))] = true
		}
	}

	var buf bytes.Buffer
	buf.WriteString(strings.TrimRight(headerBlock, "\n"))
	buf.WriteString("\r\n")
	for _, key := range SortedHeaderKeys(headers) {
		value := strings.TrimSpace(headers[key])
		if value == "" || existing[strings.ToLower(key)] {
			continue
		}
		fmt.Fprintf(&buf, "%s: %s\r\n", key, sanitizeHeaderValue(value))
	}
	buf.WriteString("\r\n")
	buf.WriteString(body)
	return buf.Bytes()
}

func formatAddress(name, address string) string {
	address = strings.TrimSpace(address)
	name = strings.TrimSpace(name)
	if name == "" {
		local := strings.SplitN(address, "@", 2)[0]
		if local == "" {
			return address
		}
		name = local
	}
	return (&mail.Address{Name: name, Address: address}).String()
}

func sanitizeHeaderValue(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}
