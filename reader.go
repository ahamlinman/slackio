package slackio

import (
	"errors"
	"io"
	"sync"
)

// ReadClient represents objects that can provide a stream of slackio Messages.
// Note that in slackio, Client implements this interface.
type ReadClient interface {
	GetMessageStream() (<-chan Message, chan<- struct{})
}

// Reader reads messages from the main body of one or more Slack channels.
type Reader struct {
	Client         ReadClient // required
	SlackChannelID string     // optional; filters by Slack channel if provided

	initOnce sync.Once
	stopOnce sync.Once
	done     chan struct{}
	wg       sync.WaitGroup
	readOut  io.ReadCloser
	readIn   io.WriteCloser
}

// init initializes internal state and spawns internal goroutines for a Reader.
// It must be called at the beginning of all other Reader methods.
func (c *Reader) init() {
	c.initOnce.Do(func() {
		if c.Client == nil {
			panic(errors.New("slackio: Client is required for Reader"))
		}

		c.done = make(chan struct{})
		c.readOut, c.readIn = io.Pipe()

		// Process incoming reads from the Client; note that the stream channel
		// will be drained until it is closed, as required by GetMessageStream.
		streamCh, streamDoneCh := c.Client.GetMessageStream()
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			defer c.stop()

			for msg := range streamCh {
				if c.SlackChannelID != "" && msg.ChannelID != c.SlackChannelID {
					continue
				}

				_, err := c.readIn.Write(append([]byte(msg.Text), byte('\n')))
				if err != nil && err != io.ErrClosedPipe {
					panic(err)
				}
			}
		}()

		// Close Client's done channel when we stop
		c.wg.Add(1)
		go func() {
			<-c.done
			close(streamDoneCh)
			c.wg.Done()
		}()
	})
}

// stop shuts down this Reader and allows internal goroutines to terminate.
func (c *Reader) stop() {
	c.stopOnce.Do(func() {
		close(c.done)

		// Closing the write half of the pipe forces Read to return EOF and Write
		// to return ErrClosedPipe. The call itself always returns nil.
		c.readIn.Close()
	})
}

// Read returns text from the main body of one or more Slack channels (i.e.
// excluding threads), buffered by line. Single messages will be terminated
// with an appended newline. Messages with explicit line breaks are equivalent
// to multiple single messages in succession.
func (c *Reader) Read(p []byte) (int, error) {
	c.init()
	return c.readOut.Read(p)
}

// Close disconnects this Reader from Slack and shuts down internal buffers.
// After calling Close, the next call to Read will result in an EOF.
func (c *Reader) Close() error {
	c.init()

	// Yes, init right before stop is dumb. But it's easier to reason about.

	c.stop()
	c.wg.Wait()
	return nil
}
