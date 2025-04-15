package media

import "time"

type PipeLineStageEffect func([]byte) error

type PipeLineStage struct {
	effect           PipeLineStageEffect
	outputBufferSize int
}

// The pipe head channel MUST be managed by the caller. Closing the channel is the
// only reliable method to tear down the pipeline. Closing the head channel causes all
// pipeline stages to return via a cascade of channel closures. After the final stage returns,
// the pipe tail will be closed.
//
// The returned error channel should be used in a select statement with the pipe tail, if an
// error is received, the caller MUST close the input channel or the pipeline stages will keep
// spinning until the source is exhausted. An error in one of the stages will not close
// the pipe tail channel, rather the errored pipeline stage will become a sink, and the tail will
// stop receiving data after subsequent buffers are emptied.
//
// When a pipeline stage errors, it enters a "sink mode" where it continues
// to consume input data at a throttled rate but does not do work or output data.
// - Prevents deadlocks by ensuring the pipeline can drain
// - Maintains backpressure to avoid overwhelming upstream stages
// - Minimizes resource usage during error state
//
// tldr; The caller must always close the pipe head input channel to ensure a proper and timely cleanup.
func NewPipeLine(pipeHead <-chan []byte, stages ...PipeLineStage) (<-chan []byte, <-chan error) {
	var pipeTail chan []byte
	cherror := make(chan error, 1)

	// just forward to output if no stages were added.
	if len(stages) == 0 {
		pipeTail = make(chan []byte, 1)

		go func() {
			defer close(pipeTail)
			for {
				b, ok := <-pipeHead

				if !ok {
					return
				}

				pipeTail <- b
			}
		}()
		return pipeTail, cherror
	}

	in := pipeHead
	for idx, stage := range stages {
		out := make(chan []byte, stage.outputBufferSize)

		if idx+1 == len(stages) {
			pipeTail = out
		}

		go func(in <-chan []byte, out chan<- []byte, f PipeLineStageEffect) {
			defer close(out)

			for {
				buf, ok := <-in

				if !ok { // closed
					return
				}

				err := f(buf)

				if err != nil {
					cherror <- err
					break
				}

				out <- buf
			}

			// sink data here until input is closed
			// has the effect of stalling further reads from the pipe tail
			// after subsequent buffers have been emptied.
			for {
				if _, ok := <-in; !ok {
					return
				}
				time.Sleep(15 * time.Millisecond)
			}

		}(in, out, stage.effect)

		in = out
	}

	return pipeTail, cherror
}
