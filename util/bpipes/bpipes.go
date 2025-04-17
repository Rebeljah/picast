package bpipes

import (
	"context"
	"errors"
	"time"

	"golang.org/x/time/rate"
)

var ErrPipelineClosing = errors.New("pipeline closing")
var ErrHeadClosed = errors.New("pipeline head channel closed")

type Stage interface {
	// A function that modifies some data and returns any unrecoverable error encountered.
	//  - The Effect function SHOULD interrupt any blocking calls upon ctx cancellation.
	//  - If the context is cancelled, the Effect function SHOULD return an errors that 'Is'
	//    context.Cancelled
	Effect(context.Context, any) error

	// The channel buffer size for this stage's output. If this is the final stage,
	// this is the buffer size of the tail of the pipe-line.
	OutputBufferSize() int

	// A non-blocking function that releases / cleans up all resources that the Stage created.
	//  - called automatically by the pipe-line on stage tear-down.
	//  - Idempotency is not required (see next bullet).
	//  - This function MUST NOT not be called from the Stage Effect func UNLESS it
	//    is idempotent AND the Effect func subsequently returns an error.
	//  - MUST NOT be called concurrently with the Effect.
	//  - The error is an unrecoverable error that either originated in the Stage Effect,
	//    or it 'Is' context.Canceled or 'Is' context.DeadlineExceeded. The error also may
	//    wrap a ctx cancellation cause, but this is usually not of concern to
	//    individual stages.
	Teardown(error)
}

type stageBase struct{}

func (s *stageBase) Effect(context.Context, any) error {
	return nil
}
func (s *stageBase) OutputBufferSize() int {
	return 0
}
func (s *stageBase) Teardown(error) {}

type ThrottlerStage struct {
	stageBase
	limiter *rate.Limiter
}

func (p *ThrottlerStage) Effect(ctx context.Context, _ any) error {
	return p.limiter.Wait(ctx)
}
func (p *ThrottlerStage) SetLimit(limit rate.Limit) {
	p.limiter.SetLimit(rate.Limit(limit))
}
func (p *ThrottlerStage) SetBurst(burst uint16) {
	p.limiter.SetBurst(int(burst))
}

// Makes a pipeline stage that can throttle the output of the previous stage to a certain freq.
// the throughput at any instant may fall below this rate, or may burst if burst > 1
func NewPipeLineThrottler(ratePerSecond rate.Limit, burst uint16) *ThrottlerStage {
	return &ThrottlerStage{
		limiter: rate.NewLimiter(ratePerSecond, int(burst)),
	}
}

type PauserStage struct {
	*ThrottlerStage
}

func (p *PauserStage) SetPaused(isPaused bool) {
	if isPaused {
		p.SetLimit(0)
	} else {
		p.SetLimit(rate.Inf)
	}
}

// A pipeline pauser is a throttler where the rate can be toggled
// between 0 and inf by the set paused function.
// The default state is paused (a rate of 0).
func NewPauserStage() *PauserStage {
	return &PauserStage{
		ThrottlerStage: NewPipeLineThrottler(0, 1),
	}
}

type SplitStage struct {
	stageBase
	splitChannel chan any
	blocking     bool
}

func (s *SplitStage) Effect(ctx context.Context, data any) error {
	if s.blocking {
		select {
		case s.splitChannel <- data:
		case <-ctx.Done():
			return errors.Join(ctx.Err(), context.Cause(ctx))
		}
	} else {
		select {
		case s.splitChannel <- data:
		default:
		}
	}

	return nil
}

func (s *SplitStage) Teardown(error) {
	close(s.splitChannel)
}

// Splits a pipeline into 2 paths by sending data to a second output channel before
// sending it the the next stage in the main pipeline. The constructor returns the SplitStage
// and it's split output channel. Notably this allows combining pipelines by using the
// output channel as the head input of a new pipeline.
//   - The output channel is closed when the stage is torn-down by its pipeline.
//   - If blocking is set true, the pipeline will stall at this stage in the event
//     that the split consumer does not empty the buffer fast enough.
//   - If blocking is set false, the split stage may skip units of data, but the
//     main pipeline will not stall as a result of a slow consumer.
func NewSplitStage(splitChannelBufferSize int, blocking bool) (*SplitStage, <-chan any) {
	stage := &SplitStage{
		splitChannel: make(chan any, splitChannelBufferSize),
		blocking:     blocking,
	}

	return stage, stage.splitChannel
}

func runPreStage[T any](ctx context.Context, head <-chan T, next chan<- T, cancelStages context.CancelCauseFunc, channelError chan<- error) {
	defer close(next)

	for {
		select {
		case <-ctx.Done():
			err := errors.Join(context.Cause(ctx), ctx.Err())
			channelError <- errors.Join(err, ErrPipelineClosing)
			return
		default:
		}

		data, ok := <-head

		// Unblock stage effects upon pipe-head closure to prevent deadlock.
		// This has the effect of causing all stage effects that honor
		// context cancellation to enter "sink" mode while waiting for their
		// previous stage to close.
		if !ok {
			err := errors.Join(ErrPipelineClosing, ErrHeadClosed)
			cancelStages(err)
			channelError <- err
			return
		}

		next <- data
	}
}

func runStageI[T any](plContext context.Context, prev <-chan T, next chan<- T, plStage Stage, channelError chan<- error) {
	var enterSinkMode bool  // controls if the stage consumes data after teardown
	var teardownCause error // is set before stage teardown

	var stageWasTornDown bool // prevents double teardown
	var nextWasClosed bool    // prevents double-close
	defer func() {
		if !nextWasClosed {
			close(next) // has priority
		}

		if !stageWasTornDown {
			plStage.Teardown(teardownCause)
		}
	}()

pumpData:
	for {
		select {
		case <-plContext.Done():
			teardownCause = errors.Join(context.Cause(plContext), plContext.Err())
			enterSinkMode = true
			break pumpData
		default:
		}

		data, ok := <-prev

		// We don't have to enter sink mode if previous stage
		// just got torn down. Immediately continue tearing down
		// the pipe-line from this stage onward; [i,n].
		if !ok {
			teardownCause = ErrPipelineClosing
			enterSinkMode = false
			break pumpData
		}

		err := plStage.Effect(plContext, data)

		if err != nil {
			if !errors.Is(err, context.Canceled) { // stage "-1" sends this error already.
				channelError <- err // SHOULD be an error originating in the Effect func, not from cancellation.
			}
			teardownCause = err
			enterSinkMode = true
			break pumpData
		}

		next <- data
	}

	close(next) // this won't be used anymore, begin teardown of stages (i,n]
	nextWasClosed = true
	plStage.Teardown(teardownCause)
	stageWasTornDown = true

	if !enterSinkMode {
		return
	}

	// sink data from i-1 buffer here until output from i-1 is closed.
	for {
		if _, ok := <-prev; !ok {
			return
		}
		time.Sleep(15 * time.Millisecond)
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
func NewPipeline[T any](ctx context.Context, pipeHead <-chan T, stages ...Stage) (<-chan T, <-chan error) {
	var pipeTail chan T
	channelError := make(chan error, 1+len(stages)) // every stage MUST (including stage "-1") be able to error without blocking

	nextInput := make(chan T, 1)

	if len(stages) == 0 {
		pipeTail = make(chan T, 1)
		nextInput = pipeTail
	}

	// pipeline-specific cancel to interrupt blocked pipeline effects
	pipeLineContext, pipeLineContextCancel := context.WithCancelCause(ctx)

	// stage "-1" just cancels the context if the input is closed to reach stopped stages.
	go runPreStage(ctx, pipeHead, nextInput, pipeLineContextCancel, channelError)

	// stages [0, n]
	for i, stage := range stages {
		currentOutput := make(chan T, stage.OutputBufferSize())

		if i+1 == len(stages) {
			pipeTail = currentOutput
		}

		go runStageI(pipeLineContext, nextInput, currentOutput, stage, channelError)

		nextInput = currentOutput
	}

	return pipeTail, channelError
}

func useexample() {
	pauser := NewPauserStage()
	pauser.SetPaused(false)
	sampler, sample := NewSplitStage(10, false)

	observablePausablePipeline, channelErr := NewPipeline(context.Background(), make(<-chan []byte), pauser, sampler)

	// run a branched pipeline concurrently using the split data as input
	go func(stageSample <-chan any) {
		thottablePipelineObserverPipeline, channelErr := NewPipeline(context.Background(), stageSample, NewPipeLineThrottler(1000, 10))
		for {
			select {
			case <-thottablePipelineObserverPipeline:
			case <-channelErr:
				return
			}
		}
	}(sample)

	for {
		select {
		case <-observablePausablePipeline:
		case <-channelErr:
			return
		}
	}
}
