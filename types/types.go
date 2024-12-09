package types

import (
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type VideoProcessingConfig struct {
	Resolutions []string
	Bitrates    []string
	AudioRates  []string
	Preset      string
	CRF         int
	SegmentTime int
}

type VideoProcessor struct {
	Logger    *slog.Logger
	S3Client  *s3.Client
	InputFile string
	OutputDir string
	S3Bucket  string
	Config    VideoProcessingConfig
}