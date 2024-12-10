package utils

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func ParseFrameRate(frameRate string) int {
	parts := strings.Split(strings.TrimSpace(frameRate), "/")
	if len(parts) == 2 {
		numerator, _ := strconv.Atoi(parts[0])
		denominator, _ := strconv.Atoi(parts[1])
		if denominator == 0 {
			denominator = 1
		}
		return numerator / denominator
	}
	return 30
}

func ParseBitrate(bitrate string) int {
	if strings.HasSuffix(bitrate, "k") {
		value, _ := strconv.Atoi(strings.TrimSuffix(bitrate, "k"))
		return value
	}
	return 1000
}

func CheckRequiredTools(logger *slog.Logger) error {
	for _, cmd := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(cmd); err != nil {
			logger.Error("Required tool not found in PATH")
			return fmt.Errorf("%s is not installed or in PATH", cmd)
		}
	}
	return nil
}

func PrepareOutputDir(outputDir string, logger *slog.Logger) error {
	if err := os.RemoveAll(outputDir); err != nil {
		logger.Error("Failed to clear output directory", "outputDir", outputDir, "error", err)
		return fmt.Errorf("failed to clear output directory: %v", err)
	}

	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		logger.Error("Failed to create output directory", "outputDir", outputDir, "error", err)
		return fmt.Errorf("failed to create output directory: %v", err)
	}
	return nil
}
