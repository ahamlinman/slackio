package slackio

import (
	"io"
	"sync"
)

// ReadClient represents objects that allow subscription to a stream of slackio
// Messages. Note that in slackio, Client implements this interface.
type ReadClient interface {
	Subscribe(chan<- Message) error
	Unsubscribe(chan<- Message) error
}

// Reader reads messages from the main body of one or more Slack channels.
type Reader struct {
	client    ReadClient
	channelID string
	msgCh     chan Message
	wg        sync.WaitGroup
	readOut   io.ReadCloser
	readIn    io.WriteCloser
}

// NewReader returns a new Reader. If channelID is non-blank, the Reader will
// only output text from a single channel. Otherwise, it will output text from
// all channels together in a single stream.
func NewReader(client ReadClient, channelID string) *Reader {
	c := &Reader{
		client:    client,
		channelID: channelID,
		msgCh:     make(chan Message, 1),
	}

	c.readOut, c.readIn = io.Pipe()
	c.client.Subscribe(c.msgCh)

	// Process incoming reads from the Client; note that the stream channel
	// will be drained until it is closed
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()

		for msg := range c.msgCh {
			if c.channelID != "" && msg.ChannelID != c.channelID {
				continue
			}

			// When this Reader is closed, this call returns an io.ErrClosedPipe.
			// This is the only possible error if we don't close readOut, and it can
			// be safely ignored.
			c.readIn.Write(append([]byte(msg.Text), byte('\n')))
		}
	}()

	return c
}

// Read returns text from the main body of one or more Slack channels (i.e.
// excluding threads), buffered by line. Single messages will be terminated
// with an appended newline. Messages with explicit line breaks are equivalent
// to multiple single messages in succession.
func (c *Reader) Read(p []byte) (int, error) {
	return c.readOut.Read(p)
}

// Close disconnects this Reader from Slack and shuts down internal buffers.
// After calling Close, the next call to Read will result in an EOF.
func (c *Reader) Close() error {
	if err := c.client.Unsubscribe(c.msgCh); err != nil {
		// This is a catastrophic situation likely indicating corruption of the
		// Client's subscription pool.
		panic(err)
	}

	// Closing the write half of the pipe forces Read to return EOF and Write
	// to return ErrClosedPipe. The call itself always returns nil.
	c.readIn.Close()
	close(c.msgCh)
	c.wg.Wait()

	return nil
}
