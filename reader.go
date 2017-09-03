package slackio

import (
	"errors"
	"io"
	"sync"
)

// ReadClient represents objects that allow subscription to a stream of slackio
// Messages. Note that in slackio, Client implements this interface.
type ReadClient interface {
	Subscribe(chan Message) error
	Unsubscribe(chan Message) error
}

// Reader reads messages from the main body of one or more Slack channels.
type Reader struct {
	Client         ReadClient // required
	SlackChannelID string     // optional; filters by Slack channel if provided

	initOnce sync.Once
	msgCh    chan Message
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

		c.readOut, c.readIn = io.Pipe()

		c.msgCh = make(chan Message, 1)
		c.Client.Subscribe(c.msgCh)

		// Process incoming reads from the Client; note that the stream channel
		// will be drained until it is closed
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()

			for msg := range c.msgCh {
				if c.SlackChannelID != "" && msg.ChannelID != c.SlackChannelID {
					continue
				}

				_, err := c.readIn.Write(append([]byte(msg.Text), byte('\n')))
				if err != nil && err != io.ErrClosedPipe {
					panic(err)
				}
			}
		}()
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
	// Yes, init right before we stop is dumb. But it's easier to reason about.
	c.init()

	if err := c.Client.Unsubscribe(c.msgCh); err != nil {
		// Should only happen if we somehow did not subscribe; currently this has
		// to be some kind of catastrophic situation.
		panic(err)
	}

	// Closing the write half of the pipe forces Read to return EOF and Write
	// to return ErrClosedPipe. The call itself always returns nil.
	c.readIn.Close()
	close(c.msgCh)
	c.wg.Wait()

	return nil
}
