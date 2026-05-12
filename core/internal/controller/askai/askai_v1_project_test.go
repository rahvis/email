package askai

import "testing"

func TestValidateProjectDomain(t *testing.T) {
	tests := []struct {
		name    string
		domain  string
		wantErr bool
	}{
		{name: "root domain", domain: "example.com"},
		{name: "subdomain", domain: "mail.example.com"},
		{name: "hyphenated", domain: "mail-server.example.com"},
		{name: "path traversal", domain: "../../etc/passwd", wantErr: true},
		{name: "slash", domain: "example.com/other", wantErr: true},
		{name: "backslash", domain: `example.com\other`, wantErr: true},
		{name: "double dot", domain: "example..com", wantErr: true},
		{name: "leading dash", domain: "-example.com", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProjectDomain(tt.domain)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
