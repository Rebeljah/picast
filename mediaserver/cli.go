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
	// use ffprobe?
	m, _ := media.ExtractMetadataFromContainerFile(cmd.Value("path").(string))
	fmt.Println(m.Genre)

	// if "format not suitable for RTP packetization (not MPEG-TS, etc)" {
	// 	// convert and save as mpeg-ts or similar
	// }

	// // index RTP packets byte offsets for effcient seek

	// // add to manifest

	return nil
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
