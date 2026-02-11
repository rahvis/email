package video_gen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultThumbnailConfig(t *testing.T) {
	cfg := DefaultThumbnailConfig("/tmp/screenshots/homepage.png", "/tmp/output")

	assert.Equal(t, "/tmp/screenshots/homepage.png", cfg.InputPath)
	assert.Equal(t, "/tmp/output/thumbnail.png", cfg.OutputPath)
	assert.Equal(t, 640, cfg.Width)
	assert.Equal(t, 360, cfg.Height)
}

func TestBuildThumbnailArgs(t *testing.T) {
	tests := []struct {
		name string
		cfg  ThumbnailConfig
		want []string
	}{
		{
			"defaults",
			ThumbnailConfig{
				InputPath:  "/tmp/input.png",
				OutputPath: "/tmp/thumb.png",
				Width:      640,
				Height:     360,
			},
			[]string{"/tmp/input.png", "-resize", "640x360!", "-quality", "90", "/tmp/thumb.png"},
		},
		{
			"custom dimensions",
			ThumbnailConfig{
				InputPath:  "/tmp/input.png",
				OutputPath: "/tmp/thumb.png",
				Width:      320,
				Height:     180,
			},
			[]string{"/tmp/input.png", "-resize", "320x180!", "-quality", "90", "/tmp/thumb.png"},
		},
		{
			"zero dimensions use defaults",
			ThumbnailConfig{
				InputPath:  "/tmp/input.png",
				OutputPath: "/tmp/thumb.png",
				Width:      0,
				Height:     0,
			},
			[]string{"/tmp/input.png", "-resize", "640x360!", "-quality", "90", "/tmp/thumb.png"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildThumbnailArgs(tt.cfg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildThumbnailArgs_InputOutputPreserved(t *testing.T) {
	cfg := ThumbnailConfig{
		InputPath:  "/path/with spaces/input.png",
		OutputPath: "/path/with spaces/thumb.png",
		Width:      640,
		Height:     360,
	}
	args := BuildThumbnailArgs(cfg)

	assert.Equal(t, "/path/with spaces/input.png", args[0])
	assert.Equal(t, "/path/with spaces/thumb.png", args[len(args)-1])
}
