package rbac

import (
	"testing"

	"billionmail-core/api/rbac/v1"
)

func TestValidateSignupReq(t *testing.T) {
	tests := []struct {
		name    string
		req     *v1.SignupReq
		wantErr bool
	}{
		{
			name: "valid signup",
			req: &v1.SignupReq{
				Username:        "ping_admin",
				Email:           "admin@example.com",
				Password:        "password123",
				ConfirmPassword: "password123",
			},
		},
		{
			name: "short username",
			req: &v1.SignupReq{
				Username:        "abc",
				Email:           "admin@example.com",
				Password:        "password123",
				ConfirmPassword: "password123",
			},
			wantErr: true,
		},
		{
			name: "invalid username characters",
			req: &v1.SignupReq{
				Username:        "ping-admin",
				Email:           "admin@example.com",
				Password:        "password123",
				ConfirmPassword: "password123",
			},
			wantErr: true,
		},
		{
			name: "invalid email",
			req: &v1.SignupReq{
				Username:        "ping_admin",
				Email:           "admin",
				Password:        "password123",
				ConfirmPassword: "password123",
			},
			wantErr: true,
		},
		{
			name: "short password",
			req: &v1.SignupReq{
				Username:        "ping_admin",
				Email:           "admin@example.com",
				Password:        "short",
				ConfirmPassword: "short",
			},
			wantErr: true,
		},
		{
			name: "mismatched password",
			req: &v1.SignupReq{
				Username:        "ping_admin",
				Email:           "admin@example.com",
				Password:        "password123",
				ConfirmPassword: "password456",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSignupReq(tt.req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateSignupReq() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeSignupReq(t *testing.T) {
	req := &v1.SignupReq{
		Username: "  ping_admin  ",
		Email:    "  admin@example.com  ",
	}

	normalizeSignupReq(req)

	if req.Username != "ping_admin" {
		t.Fatalf("username = %q", req.Username)
	}
	if req.Email != "admin@example.com" {
		t.Fatalf("email = %q", req.Email)
	}
}
