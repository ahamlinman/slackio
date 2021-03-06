package slackio

import (
	"bufio"
	"io"
	"time"
)

// Batcher is a type for functions that emit the output of an io.Reader in
// distinct "batches." Batchers may also introduce additional formatting or
// modification before an output batch is emitted.
//
// Consumers of the batcher should range over its output channel. After the
// output channel has closed, consumers should read a single error value from
// the error channel. Batcher implementations should buffer the error channel
// so that bad consumers don't create goroutine leaks.
type Batcher func(io.Reader) (<-chan string, <-chan error)

// DefaultBatcher batches lines of input over a timespan of 0.1 seconds. This
// is intended as a reasonable default for applications that occasionally write
// a chunk of multiline output.
var DefaultBatcher Batcher = NewIntervalBatcher(LineBatcher, 100*time.Millisecond, "\n")

// LineBatcher is a Batcher that emits individual, unmodified lines of output.
// Input that terminates with EOF before a newline is found will be emitted as
// if it were terminated by a newline.
func LineBatcher(r io.Reader) (<-chan string, <-chan error) {
	outCh, errCh := make(chan string), make(chan error, 1)

	go func() {
		scanner := bufio.NewScanner(r) // Breaks on newlines by default
		for scanner.Scan() {
			outCh <- scanner.Text()
		}
		close(outCh)

		errCh <- scanner.Err()
		close(errCh)
	}()

	return outCh, errCh
}

// timeAfter allows for mocking of time.After in tests. It's a slightly dirty
// implementation to make NewIntervalBatcher's interface easier to use.
var timeAfter = time.After

// NewIntervalBatcher returns a Batcher that collects the output of an upstream
// Batcher over a defined interval. When the upstream Batcher first emits an
// output batch, it is collected into a buffer and a timer is started lasting
// for the given duration. As more output is emitted while the timer is
// running, it is appended to the buffer with individual batches separated by
// the provided delimiter. When the timer expires, or when the upstream batcher
// terminates, the buffer is flushed to the output channel if it is non-blank.
// The buffer is cleared and the process repeats while the upstream batcher is
// running.
//
// The batching interval can be adjusted based on the nature of the expected
// output, though it is recommended that it be kept short.
func NewIntervalBatcher(b Batcher, d time.Duration, delim string) Batcher {
	return func(r io.Reader) (<-chan string, <-chan error) {
		inCh, inErrCh := b(r)
		outCh, outErrCh := make(chan string), make(chan error, 1)

		var output string
		var timer <-chan time.Time

		flushOutput := func() {
			if output != "" {
				outCh <- output
			}

			output = ""
		}

		go func() {
			for {
				select {
				case s, ok := <-inCh:
					if !ok {
						flushOutput()
						close(outCh)

						outErrCh <- <-inErrCh
						close(outErrCh)

						return
					}

					if output == "" {
						output = s
					} else {
						output += delim + s
					}

					if timer == nil {
						timer = timeAfter(d)
					}

				case <-timer:
					timer = nil
					flushOutput()
				}
			}
		}()

		return outCh, outErrCh
	}
}
