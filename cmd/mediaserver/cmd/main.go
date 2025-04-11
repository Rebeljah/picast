package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/joho/godotenv"

	"github.com/rebeljah/picast/http"
	"github.com/rebeljah/picast/media"
	"github.com/rebeljah/picast/mediaserver"
	"github.com/rebeljah/picast/rtp"
	"github.com/rebeljah/picast/rtsp"
	"github.com/rebeljah/picast/util/fileutil"
)

func setupLogging() (*os.File, error) {
	// Get executable path to log in the same directory
	exePath, err := os.Executable()
	if err != nil {
		return nil, err
	}

	logPath := filepath.Join(filepath.Dir(exePath), "picast.log")
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	log.SetOutput(logFile)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	return logFile, nil
}

func main() {
	// ffmpeg + ffprobe are required to be on PATH
	_, ffmpegErr := exec.LookPath("ffmpeg")
	_, ffprobeErr := exec.LookPath("ffprobe")
	if ffmpegErr != nil || ffprobeErr != nil {
		fmt.Println("could not locate ffmpeg and/or ffprobe in PATH. Verify ffmpeg installation.")
		log.Fatalln("picast media server could not locate ffmpeg/ffprobe installation")
	}

	// we will read / create data rooted at the same dir as the executable
	p, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exeDir := path.Dir(p)
	mediaDir := path.Join(exeDir, "media")
	manifestPath := path.Join(mediaDir, "manifest.json")

	// Setup logging
	logFile, err := setupLogging()
	if err != nil {
		panic(err)
	}
	defer logFile.Close()

	// Load .env from the same directory as main.go
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}

	log.Println("Starting picast application")

	var manifest media.MutableManifest
	wasCreated, err := fileutil.TouchFile(manifestPath)

	if err != nil {
		log.Fatalf("error while touching manifest file: %v", err)
	}

	if wasCreated {
		log.Printf("no manifest found at : %v, making a new one...\n", manifestPath)
		manifest = media.NewFileManifest()
	} else {
		log.Printf("Using manifest file at: %s\n", manifestPath)

		f, err := os.Open(manifestPath)
		if err != nil {
			log.Fatalf("Failed to open manifest file: %v\n", err)
		}

		manifest, err = media.NewFileManifestFromJSON(f)
		if err != nil {
			log.Fatalf("Failed to create manifest from JSON: %v\n", err)
		}
	}

	manifest.SaveJSON(manifestPath)
	defer manifest.SaveJSON(manifestPath)

	rtpServer := rtp.NewServer()
	rtspServer := rtsp.NewRTSPServer(rtpServer, manifest)
	cli := mediaserver.NewCLI(manifest)
	httpServer := http.NewServer(manifest)

	mediaserver.RunPicastMediaServer(rtspServer, rtpServer, httpServer, cli)
}
