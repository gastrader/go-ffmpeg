package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gastrader/go_ffmpeg/types"
	"github.com/gastrader/go_ffmpeg/utils"
	"github.com/joho/godotenv"
)

type VideoProcessor struct {
	Logger    *slog.Logger
	S3Client  *s3.Client
	InputFile string
	OutputDir string
	S3Bucket  string
	Config    types.VideoProcessingConfig
}

func NewVideoProcessor(logger *slog.Logger) *VideoProcessor {
	return &VideoProcessor{
		Logger: logger,
		Config: types.VideoProcessingConfig{
			Outputs:     []string{"1080", "720"},
			Resolutions: []string{"1920x1080", "1280x720"},
			Bitrates:    []string{"16000k", "6000k"},
			AudioRates:  []string{"128k", "96k"},
			Levels:      []string{"4.2", "3.1"},
			Preset:      "slow",
			CRF:         12,
			SegmentTime: 4,
		},
	}
}

func (vp *VideoProcessor) ProcessVideo() error {
	vp.Logger.Info("Processing video into segments.")

	numCPUs := runtime.NumCPU()
	sem := make(chan struct{}, numCPUs)
	var wg sync.WaitGroup
	var errChan = make(chan error, len(vp.Config.Resolutions))

	frameRateCmd := exec.Command("ffprobe", "-v", "0", "-of", "default=noprint_wrappers=1:nokey=1",
		"-select_streams", "v:0", "-show_entries", "stream=avg_frame_rate", vp.InputFile)

	frameRateOutput, err := frameRateCmd.Output()
	if err != nil {
		vp.Logger.Error("Failed to get frame rate", "error", err)
		return fmt.Errorf("failed to get frame rate: %w", err)
	}

	frameRate := utils.ParseFrameRate(string(frameRateOutput))
	gopSize := frameRate * vp.Config.SegmentTime

	for i, resolution := range vp.Config.Resolutions {
		outputName := vp.Config.Outputs[i]
		bitrate := vp.Config.Bitrates[i]
		audioRate := vp.Config.AudioRates[i]
		level := vp.Config.Levels[i]

		bitrateValue := utils.ParseBitrate(bitrate)
		maxrate := fmt.Sprintf("%dk", int(float64(bitrateValue)*1.2))
		bufsize := fmt.Sprintf("%dk", bitrateValue*2)

		playlist := filepath.Join(vp.OutputDir, fmt.Sprintf("%s.m3u8", outputName))

		sem <- struct{}{}
		wg.Add(1)

		go func(resolution, outputName, bitrate, maxrate, bufsize, playlist string) {
			defer func() {
				<-sem
				wg.Done()
			}()

			ffmpegCmd := exec.Command("ffmpeg", "-y", "-i", vp.InputFile,
				"-c:v", "libx264", "-preset", vp.Config.Preset, "-crf", "12", "-profile:v", "high", "-level:v", level,
				"-s", resolution, "-b:v", bitrate, "-maxrate", maxrate, "-bufsize", bufsize,
				"-c:a", "aac", "-b:a", audioRate, "-ac", "2",
				"-g", strconv.Itoa(gopSize), "-keyint_min", strconv.Itoa(gopSize), "-sc_threshold", "0",
				"-hls_time", "4", "-hls_list_size", "0", "-hls_flags", "independent_segments",
				"-hls_segment_filename", filepath.Join(vp.OutputDir, fmt.Sprintf("%s_%%03d.ts", outputName)),
				playlist)
			if err := ffmpegCmd.Run(); err != nil {
				vp.Logger.Error("Error processing resolution", "resolution", resolution, "error", err)
				errChan <- fmt.Errorf("error processing resolution %s: %w", resolution, err)
			}
		}(resolution, outputName, bitrate, maxrate, bufsize, playlist)
	}
	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			vp.Logger.Error("Error during video processing", "error", err)
			return fmt.Errorf("error during video processing: %w", err)
		}
	}

	masterPlaylist := filepath.Join(vp.OutputDir, "playlist.m3u8")
	vp.Logger.Info("Generating master playlist...", "masterPlaylist", masterPlaylist)

	if err := vp.GenerateMasterPlaylist(); err != nil {
		vp.Logger.Error("Failed to generate master playlist", "error", err)
		return fmt.Errorf("failed to generate master playlist: %w", err)
	}

	vp.Logger.Info("Video processing completed successfully")
	return nil
}

func (vp *VideoProcessor) UploadToS3() error {
	return filepath.Walk(vp.OutputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			vp.Logger.Error("Error walking through files", "path", path, "error", err)
			return fmt.Errorf("error walking through files: %w", err)
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(vp.OutputDir, path)
		if err != nil {
			vp.Logger.Error("Failed to calculate relative path", "path", path, "error", err)
			return fmt.Errorf("failed to calculate relative path: %w", err)
		}

		newPath := filepath.ToSlash(path)

		file, err := os.Open(path)
		if err != nil {
			vp.Logger.Error("Failed to open file", "path", path, "error", err)
			return fmt.Errorf("failed to open file %s: %w", path, err)
		}
		defer file.Close()

		_, err = vp.S3Client.PutObject(context.Background(), &s3.PutObjectInput{
			Bucket: &vp.S3Bucket,
			Key:    &newPath,
			Body:   file,
		})
		if err != nil {
			vp.Logger.Error("Failed to upload file")
			return fmt.Errorf("failed to upload file %s: %w", relPath, err)
		}
		return err
	})
}

func (vp *VideoProcessor) InitAWSClient() (*s3.Client, error) {
	err := godotenv.Load()
	if err != nil {
		return nil, fmt.Errorf("error loading .env file: %v", err)
	}

	accessKey := os.Getenv("AWS_ACCESS_KEY_ID_S3")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY_S3")
	region := os.Getenv("REGION")

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretAccessKey, "")))
	if err != nil {
		return nil, fmt.Errorf("could not load AWS config from env: %v", err)
	}
	vp.Logger.Info("S3 client initialized successfully")

	return s3.NewFromConfig(cfg), nil
}

func (vp *VideoProcessor) GenerateMasterPlaylist() error {
	masterPlaylist := filepath.Join(vp.OutputDir, "playlist.m3u8")
	vp.Logger.Info("Generating master playlist", "path", masterPlaylist)

	var buffer bytes.Buffer
	buffer.WriteString("#EXTM3U\n")
	buffer.WriteString("#EXT-X-VERSION:3\n")

	for i, playlist := range vp.Config.Outputs {
		resolution := vp.Config.Resolutions[i]
		bitrate := vp.Config.Bitrates[i]
		bandwidth := (utils.ParseBitrate(bitrate) + 128) * 1000
		buffer.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%s\n", bandwidth, resolution))
		buffer.WriteString(fmt.Sprintf("%s.m3u8\n", filepath.Base(playlist)))
	}

	return os.WriteFile(masterPlaylist, buffer.Bytes(), 0644)
}
