package utils

import (
	"log/slog"

	"github.com/gastrader/go_ffmpeg/types"
)

func NewVideoProcessor(logger *slog.Logger) *types.VideoProcessor {
	return &types.VideoProcessor{
		Logger: logger,
		Config: types.VideoProcessingConfig{
			Resolutions: []string{"1920x1080", "1280x720"},
			Bitrates:    []string{"16000k", "6000k"},
			AudioRates:  []string{"128k", "96k"},
			Preset:      "slow",
			CRF:         12,
			SegmentTime: 4,
		},
	}
}
