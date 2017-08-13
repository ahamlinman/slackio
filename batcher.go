package slackio

import (
	"bufio"
	"io"
)

// Batcher is a type for functions that emit the output of an io.Reader in
// distinct "batches." Batchers may also introduce additional formatting or
// modification before an output batch is emitted.
//
// Consumers of the batcher should range over its output channel. After the
// output channel has closed, consumers must read the emitted error value from
// the error channel.
type Batcher func(io.Reader) (<-chan string, <-chan error)

// LineBatcher is a Batcher that emits individual, unmodified lines of output.
func LineBatcher(r io.Reader) (<-chan string, <-chan error) {
	outCh := make(chan string)
	errCh := make(chan error)

	go func() {
		scanner := bufio.NewScanner(r) // Breaks on newlines by default
		for scanner.Scan() {
			outCh <- scanner.Text()
		}

		close(outCh)
		errCh <- scanner.Err()
	}()

	return outCh, errCh
}
