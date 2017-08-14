package slackio

import (
	"bufio"
	"io"
	"strings"
	"time"
)

// Batcher is a type for functions that emit the output of an io.Reader in
// distinct "batches." Batchers may also introduce additional formatting or
// modification before an output batch is emitted.
//
// Consumers of the batcher should range over its output channel. After the
// output channel has closed, consumers should read the emitted error value
// from the error channel.
type Batcher func(io.Reader) (<-chan string, <-chan error)

// DefaultBatcher batches lines of input over a timespan of 0.1 seconds. This
// is intended as a reasonable default for applications that occasionally write
// a chunk of multiline output.
var DefaultBatcher Batcher = NewIntervalBatcher(LineBatcher, 100*time.Millisecond, "\n")

// LineBatcher is a Batcher that emits individual, unmodified lines of output.
func LineBatcher(r io.Reader) (<-chan string, <-chan error) {
	outCh, errCh := make(chan string), make(chan error, 1)

	go func() {
		defer close(outCh)

		scanner := bufio.NewScanner(r) // Breaks on newlines by default
		for scanner.Scan() {
			outCh <- scanner.Text()
		}

		errCh <- scanner.Err()
	}()

	return outCh, errCh
}

// NewIntervalBatcher creates a Batcher that further batches the output of a
// Batcher over intervals of time.
//
// That is to say... when the provided Batcher emits a new batch of output, a
// timer will be started that expires after the given duration. Any output
// emitted by the provided batcher while the timer is in progress will be
// appended to previous batches, separated by the provided delimiter. When the
// timer expires, all collected output will be emitted over the output channel
// and the collection buffer will be reset. The next time the provided batcher
// emits output, the process starts again.
//
// The batching interval can be adjusted based on the nature of the expected
// output, though it is recommended that it be kept short.
func NewIntervalBatcher(b Batcher, d time.Duration, delim string) Batcher {
	return func(r io.Reader) (<-chan string, <-chan error) {
		inCh, inErrCh := b(r)
		outCh, outErrCh := make(chan string), make(chan error, 1)

		var output string
		var timer <-chan time.Time

		go func() {
			defer close(outCh)

			for {
				select {
				case s, ok := <-inCh:
					if !ok {
						return
					}

					output += delim + s

					if timer == nil {
						timer = time.After(d)
					}

				case <-timer:
					timer = nil
					outCh <- strings.TrimPrefix(output, delim)
					output = ""

				case err := <-inErrCh:
					outErrCh <- err
					return
				}
			}
		}()

		return outCh, outErrCh
	}
}
