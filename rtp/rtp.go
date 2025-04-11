package rtp

import (
	"fmt"
	"log"
	"maps"
	"net"
	"sync"

	"github.com/pion/rtp"
	"github.com/rebeljah/picast/media"
	"github.com/rebeljah/picast/rtsp"
)

type trackStream struct {
	id            rtsp.TrackStreamUID
	transportInfo rtsp.TransportInfo
	structureInfo media.StructureInfo
	trackInfo     media.TrackInfo
	stop          chan struct{}
	packetsOut    chan rtp.Packet
	raddr         *net.UDPAddr
}

func (s *trackStream) teardown() {
	close(s.packetsOut)
	close(s.stop)
}

type streams map[rtsp.TrackStreamUID]*trackStream

// implements rtsp.RTPServer
type Server struct {
	streams        streams
	interruptCause chan error
	interruptOnce  sync.Once
}

func NewServer() *Server {
	return &Server{
		streams:        make(streams),
		interruptCause: make(chan error, 1),
	}
}

func (s *Server) streamTrack(stream *trackStream) {
	defer log.Printf("RTP stream with id: %v to: %v torn down\n", stream.id, stream.raddr)
	defer s.teardownStream(stream)

	conn, err := net.DialUDP("udp", nil, stream.raddr)
	if err != nil {
		log.Printf("RTP server failed to dial udp: %v", stream.raddr)
		return
	}
	defer conn.Close()

	for {
		select {
		case <-stream.stop:
			return
		case pkt := <-stream.packetsOut:
			b, err := pkt.Marshal()
			if err != nil {
				return
			}

			_, err = conn.Write(b)

			if err != nil {
				return
			}
		}
	}
}

func (s *Server) Interrupt(err error) {
	s.interruptOnce.Do(func() {
		log.Printf("Interrupting RTP server: %v\n", err)

		for v := range maps.Values(s.streams) {
			s.teardownStream(v)
		}

		s.interruptCause <- err
	})
}

func (s *Server) SetupStream(args rtsp.SetupArguments) (rtsp.TransportInfo, error) {
	log.Printf(
		"setting up RTP stream to: %v with stream id: %v for track: (role=%v, id=%v)",
		args.RAddr, args.StreamID, args.TrackInfo.Role, args.TrackInfo.ID,
	)

	// Method SETUP not currently supported for a Ready / Playing track
	// currently, SETUP only applies to an RTSP stream in the `Init` state
	if _, ok := s.streams[args.StreamID]; ok {
		return rtsp.TransportInfo{}, fmt.Errorf("stream already exists with ID: %s", args.StreamID)
	}

	clientUDPAddr, err := net.ResolveUDPAddr("udp", args.RAddr.String())
	if err != nil {
		return rtsp.TransportInfo{}, err
	}

	selectedTransport := args.AcceptableTransports[0] // TODO: HACK! just selects most preferred without validation

	s.streams[args.StreamID] = &trackStream{
		id:            args.StreamID,
		structureInfo: args.StructureInfo,
		trackInfo:     args.TrackInfo,
		transportInfo: selectedTransport,
		stop:          make(chan struct{}),
		packetsOut:    make(chan rtp.Packet), // TODO buffer this channel?
		raddr:         clientUDPAddr,
	}

	go s.streamTrack(s.streams[args.StreamID])

	return selectedTransport, nil
}

func (s *Server) teardownStream(stream *trackStream) {
	if stream == nil {
		return
	}

	stream.teardown()
	delete(s.streams, stream.id)
}

// close the underlying connection and cleans up the stream state
//   - if the stream id is not found, this is a no-op.
func (s *Server) TeardownStream(streamID rtsp.TrackStreamUID) {
	stream, ok := s.streams[streamID]

	if !ok {
		return
	}

	s.teardownStream(stream)
}

// begin streaming
func (s *Server) PlayStream(streamID rtsp.TrackStreamUID) {
	panic("not impl")
}

func (s *Server) PauseStream(streamID rtsp.TrackStreamUID) {
	panic("not impl")
}

func (s *Server) IsServing(streamUID rtsp.TrackStreamUID) bool {
	_, ok := s.streams[streamUID]
	return ok
}

func (s *Server) InterruptCause() <-chan error {
	return s.interruptCause
}
