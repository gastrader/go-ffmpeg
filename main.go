package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/gastrader/go_ffmpeg/ffmpeg"
	"github.com/gastrader/go_ffmpeg/utils"
	"github.com/spf13/cobra"
)

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	processor := ffmpeg.NewVideoProcessor(logger)

	rootCmd := &cobra.Command{
		Use:   "video-processor [input.mp4]",
		Short: "Process video and upload HLS segments to S3",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			processor.InputFile = args[0]

			if _, err := os.Stat(processor.InputFile); os.IsNotExist(err) {
				logger.Error("Input file does not exist", "file", processor.InputFile, "error", err)
				return fmt.Errorf("input file %s does not exist", processor.InputFile)
			}

			if processor.OutputDir == "" {
				processor.OutputDir = "./output"
			}
			if err := utils.PrepareOutputDir(processor.OutputDir, logger); err != nil {
				return err
			}

			if err := utils.CheckRequiredTools(logger); err != nil {
				return err
			}

			if err := processor.ProcessVideo(); err != nil {
				logger.Error("Error processing video", "inputFile", processor.InputFile, "error", err)
				return fmt.Errorf("error processing video: %v", err)
			}

			client, err := processor.InitAWSClient()
			if err != nil {
				logger.Error("Failed to initialize AWS client", "error", err)
				return fmt.Errorf("failed to initialize AWS client: %v", err)
			}
			processor.S3Client = client

			if processor.S3Bucket != "" {
				if err := processor.UploadToS3(); err != nil {
					logger.Error("Error uploading to S3", "bucket", processor.S3Bucket, "error", err)
					return fmt.Errorf("error uploading to S3: %v", err)
				}
			}

			processor.Logger.Info("Processing and upload completed successfully.")
			return nil
		},
	}

	rootCmd.Flags().StringVarP(&processor.OutputDir, "output", "o", "", "Output directory (default: ./output)")
	rootCmd.Flags().StringVarP(&processor.S3Bucket, "bucket", "b", "", "S3 bucket to upload files")

	return rootCmd.Execute()
}
