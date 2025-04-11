package rtsp

import (
	"log"
	"slices"

	"github.com/rebeljah/picast/media"
)

// TrackState represents the RTSP track stream state.
type TrackState int

const (
	Init TrackState = iota
	Ready
	Playing
	Recording
	ErrorState
)

// Get a comprehensive TrackStreamState value representing an entire media session by
// reducing a sequence of TrackStreamState values for individual streams. The reduction follows
// the logical flow of a session involving multiple streams, e.g:
//
// A media session with 3 streams (audio.en, audio_description.en, video.h264)
// representing a movie with a separate audio description track for accessibility.
// If the session is not fully set up yet, some streams may be Ready while others
// are still in Init.
// If the TrackStreamState sequence respective to the 3 media tracks above is:
//
//	{Ready, Ready, Init}
//
// then the first 2 TrackStreamStates in the sequence can be logically reduced to a single
// Ready TrackStreamState. When this reduced state (Ready) is compared to the 3rd TrackStreamState (Init),
// the reduction becomes Init. In this specific case, the reduction must revert to Init
// because the Init state in any single stream takes precedence over any composite Ready streams.
func StateFromTrackStreamStates(streamStates []TrackState) TrackState {
	if len(streamStates) == 0 { // session not yet associated with any streams
		return Init
	}

	if len(streamStates) == 1 { // single stream (audio only, containerized a/v, etc)
		return streamStates[0]
	}

	// if any stream is in an error state, then the reduction is the error state
	if slices.Contains(streamStates, ErrorState) {
		return ErrorState
	}

	state := streamStates[0]

	// reduce state slice to a single comprehensive state value
	for _, otherState := range streamStates[1:] {
		switch state {
		case Init:
			switch otherState {
			case Init:
				state = Init
			case Ready:
				state = Init
			case Playing:
				state = ErrorState
			case Recording:
				state = ErrorState
			}
		case Ready:
			switch otherState {
			case Init:
				state = Init
			case Ready:
				state = Ready
			case Playing:
				state = ErrorState
			case Recording:
				state = ErrorState
			}
		case Playing:
			switch otherState {
			case Init:
				state = ErrorState
			case Ready:
				state = ErrorState
			case Playing:
				state = Playing
			case Recording:
				state = ErrorState
			}
		case Recording:
			switch otherState {
			case Init:
				state = ErrorState
			case Ready:
				state = ErrorState
			case Playing:
				state = ErrorState
			case Recording:
				state = Recording
			}
		default:
			state = ErrorState // TODO what to do if state is not valid option?
		}
	}

	return state
}

// identifies a specific stream for one track of an RTSP session.
// i.e streaming a movie with an audio and video stream will create a new ID
// for each stream.
type TrackStreamUID string

func newTrackStreamID() TrackStreamUID {
	return TrackStreamUID(newSessionUID(8))
}

// RTSP server state transitions based on RFC2326.
var streamStateTransitions = map[TrackState]map[RTSPMethod]TrackState{
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

func (s TrackState) After(m RTSPMethod) TrackState {
	if next, ok := streamStateTransitions[s][m]; ok {
		return next
	}
	return ErrorState
}

type TrackStreamState struct {
	media.TrackInfo
	StateNow  TrackState
	StreamUID TrackStreamUID
}

func NewTrackStreamState(track media.TrackInfo) *TrackStreamState {
	return &TrackStreamState{
		TrackInfo: track,
		StateNow:  Init,
		StreamUID: newTrackStreamID(),
	}
}

func (s *TrackStreamState) OnTeardown(ctx *requestContext) {
	s.StreamUID = ""
	s.StateNow = s.StateNow.After(TEARDOWN)
}

func (s *TrackStreamState) TransitionByMethod(m RTSPMethod) {
	prevState := s.StateNow
	s.StateNow = s.StateNow.After(m)
	log.Printf(
		"stream with uid: %v for %v track: %v changed state: %v -> %v",
		s.ID, s.TrackInfo.Role, s.TrackInfo.ID, prevState, s.StateNow,
	)
}
