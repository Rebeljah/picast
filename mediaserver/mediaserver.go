package mediaserver

import (
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/oklog/run"
	"github.com/rebeljah/picast/http"
	"github.com/rebeljah/picast/rtp"
	"github.com/rebeljah/picast/rtsp"
)

func RunPicastMediaServer(rtspServer *rtsp.RTSPServer, rtpServer *rtp.Server, httpServer *http.Server, cli *CLI) {
	var rg run.Group

	// add actors

	// os signal handler to gracefully trigger rungroup interrupt on SIGINT or SIGTERM
	signalTrap := make(chan os.Signal, 1)
	signal.Notify(signalTrap, syscall.SIGINT, syscall.SIGTERM)
	rg.Add(
		func() error {
			if sig, ok := <-signalTrap; ok {
				log.Printf("picast server rungroup interrupt due to: %v", sig)
				return errors.New(sig.String() + " signal")
			}

			return nil
		},
		func(error) {
			signal.Stop(signalTrap)
			close(signalTrap)
		},
	)

	// RTSP server
	rg.Add(
		func() error {
			return rtspServer.ListenAndServe("localhost:8554")
		},
		rtspServer.Interrupt,
	)

	// HTTP server
	rg.Add(
		func() error {
			return httpServer.ListenAndServe("localhost:8080")
		},
		httpServer.Interrupt,
	)

	// RTP server
	rg.Add(
		func() error {
			// RTP server won't begin work until triggered by the RTSP server, so
			// there is no proccess to kick of here. We just wait here until
			// the RTP server has been shutdown.
			return <-rtpServer.InterruptCause()
		},
		rtpServer.Interrupt,
	)

	// CLI
	rg.Add(cli.Run, cli.Interrupt)

	log.Println("Starting server group")
	err := rg.Run()
	log.Printf("Server group exited: %v\n", err)
}
