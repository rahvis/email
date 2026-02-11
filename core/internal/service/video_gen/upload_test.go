package video_gen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultR2Config(t *testing.T) {
	cfg := DefaultR2Config()
	// Returns empty strings when env vars not set
	assert.IsType(t, R2Config{}, cfg)
}

func TestR2Endpoint(t *testing.T) {
	tests := []struct {
		name      string
		accountID string
		want      string
	}{
		{"standard", "abc123", "https://abc123.r2.cloudflarestorage.com"},
		{"empty", "", "https://.r2.cloudflarestorage.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, R2Endpoint(tt.accountID))
		})
	}
}

func TestBuildR2ObjectKey(t *testing.T) {
	tests := []struct {
		name      string
		contactID string
		filename  string
		want      string
	}{
		{"video", "contact-123", "video.mp4", "video-outreach/contact-123/video.mp4"},
		{"thumbnail", "contact-456", "thumb.png", "video-outreach/contact-456/thumb.png"},
		{"nested", "abc", "subdir/file.mp4", "video-outreach/abc/subdir/file.mp4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, BuildR2ObjectKey(tt.contactID, tt.filename))
		})
	}
}

func TestBuildPublicURL(t *testing.T) {
	tests := []struct {
		name string
		cfg  R2Config
		key  string
		want string
	}{
		{
			"trailing slash",
			R2Config{PublicURL: "https://cdn.example.com/"},
			"video-outreach/c1/video.mp4",
			"https://cdn.example.com/video-outreach/c1/video.mp4",
		},
		{
			"no trailing slash",
			R2Config{PublicURL: "https://cdn.example.com"},
			"video-outreach/c1/thumb.png",
			"https://cdn.example.com/video-outreach/c1/thumb.png",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, BuildPublicURL(tt.cfg, tt.key))
		})
	}
}

func TestDetectContentType(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"video.mp4", "video/mp4"},
		{"video.webm", "video/webm"},
		{"image.png", "image/png"},
		{"image.jpg", "image/jpeg"},
		{"image.jpeg", "image/jpeg"},
		{"image.gif", "image/gif"},
		{"audio.wav", "audio/wav"},
		{"audio.mp3", "audio/mpeg"},
		{"unknown.xyz", "application/octet-stream"},
		{"VIDEO.MP4", "video/mp4"},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			assert.Equal(t, tt.want, detectContentType(tt.filename))
		})
	}
}

func TestBuildLandingPageURL(t *testing.T) {
	tests := []struct {
		name         string
		baseURL      string
		videoURL     string
		thumbnailURL string
		contactName  string
		want         string
	}{
		{
			"standard",
			"https://douro-digital-agency.vercel.app/book-a-call",
			"https://cdn.example.com/video.mp4",
			"https://cdn.example.com/thumb.png",
			"John",
			"https://douro-digital-agency.vercel.app/book-a-call?video=https://cdn.example.com/video.mp4&thumb=https://cdn.example.com/thumb.png&name=John",
		},
		{
			"trailing slash",
			"https://example.com/page/",
			"https://cdn.example.com/v.mp4",
			"https://cdn.example.com/t.png",
			"Jane",
			"https://example.com/page?video=https://cdn.example.com/v.mp4&thumb=https://cdn.example.com/t.png&name=Jane",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildLandingPageURL(tt.baseURL, tt.videoURL, tt.thumbnailURL, tt.contactName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewR2Client_NotNil(t *testing.T) {
	cfg := R2Config{
		AccountID:       "test-account",
		AccessKeyID:     "test-key",
		AccessKeySecret: "test-secret",
		BucketName:      "test-bucket",
		PublicURL:       "https://cdn.example.com",
	}
	client := NewR2Client(cfg)
	assert.NotNil(t, client)
}
