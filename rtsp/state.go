package rtsp

// StreamStateName represents the RTSP track stream state.
type StreamStateName int

const (
	Init StreamStateName = iota
	Ready
	Playing
	Recording
	ErrorState
)

// identifies a specific stream for one track of an RTSP session.
// i.e streaming a movie with an audio and video stream will create a new ID
// for each stream.
type StreamUID string

func newStreamUID() StreamUID {
	return StreamUID(newSessionUID(8))
}

// RTSP server state transitions based on RFC2326.
var streamStateTransitions = map[StreamStateName]map[RTSPMethod]StreamStateName{
	Init: {
		SETUP:    Ready,
		TEARDOWN: Init,
	},
	Ready: {
		PLAY:     Playing,
		RECORD:   Recording,
		TEARDOWN: Init,
		SETUP:    Ready,
	},
	Playing: {
		PLAY:     Playing,
		PAUSE:    Ready,
		TEARDOWN: Init,
		SETUP:    Playing,
	},
	Recording: {
		RECORD:   Recording,
		PAUSE:    Ready,
		TEARDOWN: Init,
		SETUP:    Recording,
	},
}

func (s StreamStateName) After(m RTSPMethod) StreamStateName {
	if next, ok := streamStateTransitions[s][m]; ok {
		return next
	}
	return ErrorState
}

type StreamState struct {
	StateNow  StreamStateName
	StreamUID StreamUID
}

func NewStreamState() *StreamState {
	return &StreamState{
		StateNow:  Init,
		StreamUID: newStreamUID(),
	}
}

func (s *StreamState) OnTeardown() {
	s.StreamUID = ""
	s.StateNow = s.StateNow.After(TEARDOWN)
}

func (s *StreamState) OnSetup() {
	s.StateNow = s.StateNow.After(SETUP)
}
