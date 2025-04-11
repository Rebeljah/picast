package mediaserver

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/rebeljah/picast/media"
	"github.com/urfave/cli/v3"
)

type ErrReadCancelled struct {
	cause error
}

func (e ErrReadCancelled) Error() string { return "read cancelled" }
func (e ErrReadCancelled) Unwrap() error { return e.cause }

var errReadCancelled ErrReadCancelled

var errExitFromCLI = errors.New("CLI exit")

type CancelableReader struct {
	cancel <-chan error
	data   chan []byte
	err    error
	r      io.Reader
}

func (c *CancelableReader) begin() {
	buf := make([]byte, 1024)
	for {
		n, err := c.r.Read(buf)
		if n > 0 {
			tmp := make([]byte, n)
			copy(tmp, buf[:n])
			c.data <- tmp
		}
		if err != nil {
			c.err = err
			close(c.data)
			return
		}
	}
}

func (c *CancelableReader) Read(p []byte) (int, error) {
	select {
	case err := <-c.cancel:
		return 0, ErrReadCancelled{cause: err}
	case d, ok := <-c.data:
		if !ok {
			return 0, c.err
		}
		copy(p, d)
		return len(d), nil
	}
}

func NewCancelableReader(cancel <-chan error, r io.Reader) *CancelableReader {
	c := &CancelableReader{
		cancel: cancel,
		r:      r,
		data:   make(chan []byte),
	}
	go c.begin()
	return c
}

type CLI struct {
	manifest      media.MutableManifest
	reader        *CancelableReader
	cancelReader  chan<- error
	interruptOnce sync.Once
}

func (c *CLI) commandMediaAdd(ctx context.Context, cmd *cli.Command) error {
	// given the path to a standalone or container media (mkv, mp3, mpeg-ts, mp4, etc),
	// move/copy the file into the media storage. The file will be recoded into a
	// RTP-friendly container codec
	//////////

	// get ffprobe data
	// if the container is not RTP friednly, use ffmpeg to re-encode (save metadata like disposition!!)

	// create a Metadata struct for the file
	// - The media add command only supports adding a single file, not directories
	//  composed of multiple element files.
	// - The returned Metadata structure will contain exactly 1 top-level track which
	//   either represents a container, or standalone media (e.g music).
	// - The top-level track is a container (e.g mp4) iff it contains 2 or more
	//   tracks representing individual elements (e.g aac, avc)
	// - CLI users may add additional top-level tracks at a later time (e.g
	//   an audio description)
	// - Additional top-level tracks added later MUST be audio and/or subtitles (the
	//   behavior of multiple video tracks is undefined as of now).

	pName := cmd.Value("path").(string)
	outputName := pName + ".ts" // Output will always be TS

	log.Println("Converting media to RTP-optimized MPEG-TS format...")

	ffmpegArgs := []string{
		"-i", pName, // Input file name/path

		// Video Stream Selection
		"-map", "0:v:0?", // Keep first video stream if exists (0:v:0 with ? makes it optional)

		// Audio Stream Selection
		"-map", "0:a?", // Keep all audio streams if any exist (0:a with ? makes it optional)

		// Video Encoding Settings (applied if video exists)
		"-c:v", "libx264", // Use H.264 video codec
		"-preset", "fast", // Faster encoding with slightly larger file size
		"-tune", "zerolatency", // Optimize for low latency streaming
		"-b:v", "4000k", // Target video bitrate (4000 kbps)
		"-maxrate", "4000k", // Maximum video bitrate
		"-minrate", "4000k", // Minimum video bitrate
		"-bufsize", "8000k", // Ratecontrol buffer size (2x target bitrate)
		"-x264-params", "nal-hrd=cbr:keyint=60:min-keyint=60", // Force CBR, GOP length=60 frames
		"-pix_fmt", "yuv420p", // Standard pixel format for compatibility

		// Audio Encoding Settings (applied if audio exists)
		"-c:a", "aac", // Use AAC audio codec
		"-b:a", "256k", // Audio bitrate (256 kbps)

		// Metadata Handling
		"-map_metadata", "0", // Copy all metadata from input to output

		// MPEG-TS Output Settings
		"-f", "mpegts", // Force MPEG-TS output format
		"-mpegts_flags", "no_rtcp", // Disable RTCP to reduce overhead
		"-flags", "+global_header", // Add global headers for some streaming protocols
		"-y",       // Overwrite output file without asking
		outputName, // Output file name/path
	}

	// TODO: Execute ffmpeg command
	log.Printf("Would execute: ffmpeg %s", strings.Join(ffmpegArgs, " "))

	// TODO: Index RTP packets byte offsets for efficient seek
	// TODO: Add to manifest
}

func NewCLI(manifest media.MutableManifest) *CLI {
	c := make(chan error, 1)

	return &CLI{
		manifest:     manifest,
		reader:       NewCancelableReader(c, os.Stdin),
		cancelReader: c,
	}
}

func (c *CLI) Run() error {
	log.Println("running picast CLI")
	defer log.Println("picast CLI stopped")

	// override default error handler (we don't want to exit on error)
	cli.OsExiter = func(int) {}

	cmd := &cli.Command{
		Commands: []*cli.Command{
			{
				Name:    "media",
				Aliases: []string{"m"},
				Usage:   "Manage media hosted on the picast server",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "id",
						Aliases: []string{"i"},
						Usage:   "Unique ID for media hosted on the server",
					},
				},
				Commands: []*cli.Command{
					{
						Name:  "add",
						Usage: "add music or video to the media server",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:      "path",
								Aliases:   []string{"p"},
								Usage:     "the path, either absolute, or relative to your cwd, of the media file to add to the media server",
								TakesFile: true,
								Required:  true,
							},
						},
						Action: c.commandMediaAdd,
					},
					{
						Name:  "remove",
						Usage: "remove music or video from the media server",
					},
					{
						Name:  "edit",
						Usage: "edit music or video metadata hosted on the media server",
					},
					{
						Name:  "list",
						Usage: "list music and/or video hosted on the media server",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:    "music",
								Aliases: []string{"m"},
								Value:   true,
							},
							&cli.BoolFlag{
								Name:    "video",
								Aliases: []string{"v"},
								Value:   true,
							},
						},
					},
				},
			},
			{
				Name: "exit",
				Action: func(context.Context, *cli.Command) error {
					c.Interrupt(errExitFromCLI)
					return nil
				},
			},
		},
	}

	reader := bufio.NewReader(c.reader)
	for {
		fmt.Print("picast> ")

		input, err := reader.ReadString('\n')
		if err != nil {
			// If the input read was cancelled on purpose, we are more interested in the
			// root cause (usually due to CLI exit or shutdown of a server)
			if errors.As(err, &errReadCancelled) {
				return errors.Unwrap(err)
			}
			return err
		}

		input = strings.TrimSpace(input)

		// Split input into args and prepend the program name
		args := append([]string{"picast"}, strings.Fields(input)...)
		if err := cmd.Run(context.Background(), args); err != nil {
			log.Println(err)
		}
	}
}

func (c *CLI) Interrupt(cause error) {
	c.interruptOnce.Do(func() {
		log.Printf("stopping picast CLI: %v\n", cause)

		c.cancelReader <- cause
	})
}
