
# FFmpeg to S3 CLI

This CLI tool processes videos and uploads HLS segments to an S3 bucket using FFmpeg. The tool supports segmenting videos into multiple resolutions and bitrates, and optionally uploading the resulting files to S3 for storage and streaming.

## Prerequisites

Before running the application, make sure you have the following tools installed on your system:

- **FFmpeg**: A command-line tool for processing video and audio files.
- **Go (1.18 or newer)**: The Go programming language to build the CLI application.

You also need the following environment variables to configure the application:

### Environment Variables

Create a `.env` file in the root of your project and define the following environment variables:

1. **`AWS_ACCESS_KEY_ID`**: Your AWS access key for authentication with AWS services.
2. **`AWS_SECRET_ACCESS_KEY`**: Your AWS secret key for authentication with AWS services.
3. **`AWS_REGION`**: The AWS region where your S3 bucket is located (e.g., `us-east-1`).

## Building the Application

1. Clone the repository or copy the project to your local machine.

2. Make sure you have Go installed. If you don't have Go, you can download and install it from [here](https://golang.org/dl/).

3. Install any dependencies:

   ```bash
   go mod tidy
   ```

4. Build the executable:

   ```bash
   go build -o video-processor main.go
   ```

This will create a binary file named `video-processor` in your current directory.

## Running the Application

Once the application is built and your `.env` file is configured, you can run the application using the following command:

```bash
./video-processor [input.mp4]
```

Replace `[input.mp4]` with the path to the video file you want to process.

### Available Flags

- **`-o` or `--output`**: Specify the output directory for the processed video segments (default is `./output`).
  
  Example:

  ```bash
  ./video-processor --output ./processed /path/to/video.mp4
  ```

- **`-b` or `--bucket`**: Specify the S3 bucket to upload the processed files to.

  Example:

  ```bash
  ./video-processor --bucket my-s3-bucket /path/to/video.mp4
  ```

## Workflow

The `video-processor` will:

1. **Process the video**: Using FFmpeg, the video will be processed into multiple segments based on the resolutions and bitrates defined in the `VideoProcessor` configuration.
   
2. **Generate playlists**: After segmenting the video, it generates a master playlist (`playlist.m3u8`) and individual resolution-specific playlists (e.g., `video_1280x720.m3u8`).

3. **Upload to S3**: If an S3 bucket is provided, the video segments and playlists will be uploaded to the specified S3 bucket.

### Command:

```bash
./video-processor -o ./output -b my-s3-bucket /path/to/input.mp4
```

