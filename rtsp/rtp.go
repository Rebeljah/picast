package rtsp

import (
	"net"

	"gopkg.in/vansante/go-ffprobe.v2"
)

// RTPServer defines what RTSP needs from the RTP implementation
type RTPServer interface {
	SetupStream(SetupArguments) (TransportInfo, error)
	TeardownStream(StreamUID)
	PlayStream(StreamUID)
	PauseStream(StreamUID)
	Interrupt(error)
	InterruptCause() <-chan error
}

type SetupArguments struct {
	StreamID             StreamUID
	RAddr                net.Addr
	AcceptableTransports []TransportInfo
	Spec                 ffprobe.ProbeData
}

func newSetupArguments(
	streamID StreamUID,
	clientAddr net.Addr,
	spec ffprobe.ProbeData,
	acceptableTransports []TransportInfo,
) SetupArguments {
	return SetupArguments{
		StreamID:             streamID,
		RAddr:                clientAddr,
		Spec:                 spec,
		AcceptableTransports: acceptableTransports,
	}
}
