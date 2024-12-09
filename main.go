package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	var (
		outputDir string
		s3Bucket  string
	)
	rootCmd := &cobra.Command{
		Use:   "video-processor [input.mp4]",
		Short: "Process video and upload HLS segments to S3",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inputFile := args[0]

			if _, err := os.Stat(inputFile); os.IsNotExist(err) {
				return fmt.Errorf("input file %s does not exist", inputFile)
			}

			if outputDir == "" {
				outputDir = "./output"
			}

			if err := os.RemoveAll(outputDir); err != nil {
				return fmt.Errorf("failed to clear output directory: %v", err)
			}

			if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
				return fmt.Errorf("failed to create output directory: %v", err)
			}

			// Check required commands
			for _, cmd := range []string{"ffmpeg", "ffprobe"} {
				if _, err := exec.LookPath(cmd); err != nil {
					return fmt.Errorf("%s is not installed or in PATH", cmd)
				}
			}

			if err := processVideo(inputFile, outputDir); err != nil {
				return fmt.Errorf("error processing video: %v", err)
			}

			client, err := initAWSClient()
			if err != nil {
				log.Fatalf("Failed to initialize AWS client: %v", err)
			}

			if s3Bucket != "" {
				if err := uploadToS3(outputDir, s3Bucket, client); err != nil {
					return fmt.Errorf("error uploading to S3: %v", err)
				}
			}

			fmt.Println("Processing and upload completed successfully.")
			return nil
		},
	}

	rootCmd.Flags().StringVarP(&outputDir, "output", "o", "", "Output directory (default: ./output)")
	rootCmd.Flags().StringVarP(&s3Bucket, "bucket", "b", "", "S3 bucket to upload files")

	return rootCmd.Execute()
}

func processVideo(inputFile, outputDir string) error {
	resolutions := []string{"1920x1080", "1280x720"}
	bitrates := []string{"16000k", "6000k"}
	outputs := []string{"1080p", "720p"}
	audioRates := []string{"128k", "96k"}
	var playlists []string

	fmt.Println("Processing video into segments.")

	// Get frame rate using ffprobe
	frameRateCmd := exec.Command("ffprobe", "-v", "0", "-of", "default=noprint_wrappers=1:nokey=1",
		"-select_streams", "v:0", "-show_entries", "stream=avg_frame_rate", inputFile)
	frameRateOutput, err := frameRateCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get frame rate: %v", err)
	}
	frameRate := parseFrameRate(string(frameRateOutput))
	gopSize := frameRate * 4

	for i, resolution := range resolutions {
		outputName := outputs[i]
		bitrate := bitrates[i]
		audioRate := audioRates[i]

		bitrateValue := parseBitrate(bitrate)
		maxrate := fmt.Sprintf("%dk", int(float64(bitrateValue)*1.2))
		bufsize := fmt.Sprintf("%dk", bitrateValue*2)

		playlist := filepath.Join(outputDir, fmt.Sprintf("%s.m3u8", outputName))
		playlists = append(playlists, playlist)

		// Run ffmpeg
		ffmpegCmd := exec.Command("ffmpeg", "-y", "-i", inputFile,
			"-c:v", "libx264", "-preset", "slow", "-crf", "12", "-profile:v", "high", "-level:v", "4.2",
			"-s", resolution, "-b:v", bitrate, "-maxrate", maxrate, "-bufsize", bufsize,
			"-c:a", "aac", "-b:a", audioRate, "-ac", "2",
			"-g", strconv.Itoa(gopSize), "-keyint_min", strconv.Itoa(gopSize), "-sc_threshold", "0",
			"-hls_time", "4", "-hls_list_size", "0", "-hls_flags", "independent_segments",
			"-hls_segment_filename", filepath.Join(outputDir, fmt.Sprintf("%s_%%03d.ts", outputName)),
			playlist)
		if err := ffmpegCmd.Run(); err != nil {
			return fmt.Errorf("error processing %s: %v", resolution, err)
		}
	}

	// Generate master playlist
	masterPlaylist := filepath.Join(outputDir, "playlist.m3u8")
	if err := generateMasterPlaylist(masterPlaylist, resolutions, playlists, bitrates); err != nil {
		return err
	}

	return nil
}

func parseFrameRate(frameRate string) int {
	parts := strings.Split(strings.TrimSpace(frameRate), "/")
	if len(parts) == 2 {
		numerator, _ := strconv.Atoi(parts[0])
		denominator, _ := strconv.Atoi(parts[1])
		if denominator == 0 {
			denominator = 1
		}
		return numerator / denominator
	}
	return 30 // Default frame rate
}

func generateMasterPlaylist(masterPlaylist string, resolutions, playlists, bitrates []string) error {
	var buffer bytes.Buffer
	buffer.WriteString("#EXTM3U\n")
	buffer.WriteString("#EXT-X-VERSION:3\n")

	for i, playlist := range playlists {
		resolution := resolutions[i]
		bitrate := bitrates[i]
		bandwidth := (parseBitrate(bitrate) + 128) * 1000
		buffer.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%s\n", bandwidth, resolution))
		buffer.WriteString(fmt.Sprintf("%s\n", filepath.Base(playlist)))
	}

	return os.WriteFile(masterPlaylist, buffer.Bytes(), 0644)
}

func parseBitrate(bitrate string) int {
	if strings.HasSuffix(bitrate, "k") {
		value, _ := strconv.Atoi(strings.TrimSuffix(bitrate, "k"))
		return value
	}
	return 1000
}

func uploadToS3(outputDir, bucket string, client *s3.Client) error {
	return filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error walking through files: %v", err)
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(outputDir, path)
		if err != nil {
			return fmt.Errorf("failed to calculate relative path: %v", err)
		}

		newPath := filepath.ToSlash(path)

		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open file: %v", err)
		}
		defer file.Close()

		_, err = client.PutObject(context.Background(), &s3.PutObjectInput{
			Bucket: &bucket,
			Key:    &newPath,
			Body:   file,
		})
		if err != nil {
			return fmt.Errorf("failed to upload file %s: %v", relPath, err)
		}
		return err
	})
}

func initAWSClient() (*s3.Client, error) {
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
	fmt.Println("S3 client initialized successfully")

	return s3.NewFromConfig(cfg), nil
}
