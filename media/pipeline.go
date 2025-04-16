package media

import (
	"context"
	"errors"
	"time"

	"golang.org/x/time/rate"
)

var ErrPipeLineClosing = errors.New("pipeline closing")

type PipeLineStage interface {
	// A function that modifies a byte slice's contents and returns any error encountered.
	//  - The Effect function SHOULD interrupt any blocking calls upon ctx cancellation.
	//  - If the context is cancelled, the Effect function SHOULD return an errors that 'Is'
	//    context.Cancelled
	Effect(context.Context, []byte) error

	// The channel buffer size for this stage's output. If this is the final stage,
	// this is the buffer size of the tail of the pipe-line.
	OutputBufferSize() int
}

type PipeLineThrottler struct {
	limiter *rate.Limiter
}

func (p *PipeLineThrottler) Effect(ctx context.Context, _ []byte) error {
	return p.limiter.Wait(ctx)
}
func (p *PipeLineThrottler) OutputBufferSize() int {
	return 0
}
func (p *PipeLineThrottler) SetLimit(limit rate.Limit) {
	p.limiter.SetLimit(rate.Limit(limit))
}
func (p *PipeLineThrottler) SetBurst(burst uint16) {
	p.limiter.SetBurst(int(burst))
}

// Makes a pipeline stage that can throttle the output of the previous stage to a certain freq.
// the throughput at any instant may fall below this rate, or may burst if burst > 1
func NewPipeLineThrottler(ratePerSecond rate.Limit, burst uint16) *PipeLineThrottler {
	return &PipeLineThrottler{
		limiter: rate.NewLimiter(ratePerSecond, int(burst)),
	}
}

type PipeLinePauser struct {
	*PipeLineThrottler
}

func (p *PipeLinePauser) SetPaused(isPaused bool) {
	if isPaused {
		p.SetLimit(0)
	} else {
		p.SetLimit(rate.Inf)
	}
}

// A pipeline pauser is a throttler where the rate can be toggled
// between 0 and inf by the set paused function.
// The default state is paused (a rate of 0).
func NewPipeLinePauser() *PipeLinePauser {
	return &PipeLinePauser{
		PipeLineThrottler: NewPipeLineThrottler(0, 1),
	}
}

// Creates a concurrent pipeline that autoamtically manages all goroutines and
// channels needed. The caller is only responsible for closing the pipe-head — it will
// never automatically close as a result of calling this function. The caller MUST
// tear down the pipe-line by either cancelling its context, or by closing the pipe-head.
//
// The function returns a channel representing the tail of the pipe-line, and a channel
// that will send errors from any pipe-line stage effect.
//
// If a stage effect encounters any error, it will first send the error to the error
// channel and begin a teardown of its subsequent pipe-line stages by closing its
// own output channel. Next, the errored stage will enter "sink" mode. The pipe-line
// will continue to pull data from the pipe-head and sink it at the errored stage
// until the pipe-head is closed OR the pipe-line context is cancelled. The stage will
// continue to consume input data from the previous stage at a throttled rate but
// does not do work or output data.
//   - Prevents deadlocks by ensuring the pipeline can drain
//   - Maintains backpressure to avoid overwhelming upstream stages
//   - Minimizes resource usage during error state
func NewPipeLine(ctx context.Context, pipeHead <-chan []byte, stages ...PipeLineStage) (<-chan []byte, <-chan error) {
	var pipeTail chan []byte
	channelError := make(chan error, 1+len(stages)) // every stage MUST (including stage "-1") be able to error without blocking

	nextInput := make(chan []byte, 1)

	if len(stages) == 0 {
		pipeTail = make(chan []byte, 1)
		nextInput = pipeTail
	}

	// pipeline-specific cancel to interrupt blocked pipeline effects
	pipeLineContext, pipeLineContextCancel := context.WithCancelCause(ctx)

	// stage "-1" just cancels the context if the input is closed to reach stopped stages.
	go func(head <-chan []byte, next chan<- []byte, ctx context.Context, cancelStages context.CancelCauseFunc) {
		defer close(next)

		for {
			select {
			case <-ctx.Done():
				cancelStages(ErrPipeLineClosing)
				channelError <- errors.Join(context.Cause(ctx), ctx.Err(), ErrPipeLineClosing)
				return
			default:
			}

			buf, ok := <-head

			// Unblock stage effects upon pipe-head closure to prevent deadlock.
			// This has the effect of causing all stage effects that honor
			// context cancellation to enter "sink" mode while waiting for their
			// previous stage to close.
			if !ok {
				cancelStages(ErrPipeLineClosing)
				channelError <- ErrPipeLineClosing
				return
			}

			next <- buf
		}
	}(pipeHead, nextInput, ctx, pipeLineContextCancel)

	// stages [0, n]
	for i, stage := range stages {
		currentOutput := make(chan []byte, stage.OutputBufferSize())

		if i+1 == len(stages) {
			pipeTail = currentOutput
		}

		go func(ctx context.Context, in <-chan []byte, out chan<- []byte, plStage PipeLineStage) {
			defer close(out)

		pumpData:
			for {
				select {
				case <-ctx.Done():
					break pumpData
				default:
				}

				buf, ok := <-in

				// We don't have to enter sink mode if previous stage
				// just got torn down. Immediately continue tearing down
				// the pipe-line from this stage onward; [i,n].
				if !ok {
					return
				}

				err := plStage.Effect(ctx, buf)

				if err != nil {
					if !errors.Is(err, errors.Join(context.Canceled, ErrPipeLineClosing)) { // caller should already know that the ctx cancelled.
						channelError <- err // SHOULD be an error originating in the Effect func, not from cancellation.
					}
					break pumpData
				}

				out <- buf
			}

			close(out) // this won't be used anymore, begin teardown of stages (i,n]

			// sink data from i-1 buffer here until output from i-1 is closed.
			for {
				if _, ok := <-in; !ok {
					return
				}
				time.Sleep(15 * time.Millisecond)
			}

		}(pipeLineContext, nextInput, currentOutput, stage)

		nextInput = currentOutput
	}

	return pipeTail, channelError
}
