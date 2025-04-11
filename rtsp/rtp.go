package rtsp

import (
	"net"

	"github.com/rebeljah/picast/media"
)

// RTPServer defines what RTSP needs from the RTP implementation
type RTPServer interface {
	SetupStream(SetupArguments) (TransportInfo, error)
	TeardownStream(TrackStreamUID)
	PlayStream(TrackStreamUID)
	PauseStream(TrackStreamUID)
	Interrupt(error)
	InterruptCause() <-chan error
}

type SetupArguments struct {
	StreamID             TrackStreamUID
	RAddr                net.Addr
	StructureInfo        media.StructureInfo
	TrackInfo            media.TrackInfo
	AcceptableTransports []TransportInfo
}

func newSetupArguments(
	streamID TrackStreamUID,
	clientAddr net.Addr,
	structureInfo media.StructureInfo,
	trackInfo media.TrackInfo,
	acceptableTransports []TransportInfo,
) SetupArguments {
	return SetupArguments{
		StreamID:             streamID,
		RAddr:                clientAddr,
		StructureInfo:        structureInfo,
		TrackInfo:            trackInfo,
		AcceptableTransports: acceptableTransports,
	}
}
