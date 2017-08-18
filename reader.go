package slackio

import (
	"errors"
	"io"
	"sync"
)

// Reader reads messages from the main body of one or more Slack channels.
type Reader struct {
	Client         *Client // required
	SlackChannelID string  // optional; filters by Slack channel if provided

	initOnce sync.Once
	wg       sync.WaitGroup
	done     chan struct{}
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

		// Process incoming reads from the Client
		streamCh, streamDoneCh := c.Client.GetMessageStream()
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			for msg := range streamCh {
				if c.SlackChannelID != "" && msg.ChannelID != c.SlackChannelID {
					continue
				}

				if _, err := c.readIn.Write(append([]byte(msg.Text), byte('\n'))); err != nil {
					panic(err)
				}
			}
		}()

		// Close Client's done channel when we close ours
		c.wg.Add(1)
		go func() {
			<-c.done
			close(streamDoneCh)
			c.wg.Done()
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
	c.init()

	close(c.done)
	c.readIn.Close() // Always returns nil
	c.wg.Wait()
	return nil
}
