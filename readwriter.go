// Package slackio implements real-time communication with a single Slack
// channel behind an io.ReadWriteCloser interface.
package slackio

import (
	"errors"
	"io"
	"sync"
)

// ReadWriter is a ReadWriteCloser for a single Slack channel, where the
// content of the channel's main body is represented as lines of text.
type ReadWriter struct {
	Client         *Client // required
	SlackChannelID string  // required
	Batcher        Batcher

	initOnce sync.Once
	wg       sync.WaitGroup
	done     chan struct{}
	readOut  io.ReadCloser
	readIn   io.WriteCloser
	writeOut io.ReadCloser
	writeIn  io.WriteCloser
}

// init initializes internal state for a ReadWriter and spawns internal
// goroutines.  It must be called at the beginning of all other ReadWriter
// methods.
func (c *ReadWriter) init() {
	c.initOnce.Do(func() {
		if c.Client == nil || c.SlackChannelID == "" {
			panic(errors.New("Streamer and SlackChannelID are required"))
		}

		if c.Batcher == nil {
			c.Batcher = DefaultBatcher
		}

		c.done = make(chan struct{})
		c.readOut, c.readIn = io.Pipe()
		c.writeOut, c.writeIn = io.Pipe()

		// Process incoming reads from the Streamer
		streamCh, streamDoneCh := c.Client.GetMessageStream()
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			for msg := range streamCh {
				if msg.ChannelID != c.SlackChannelID {
					continue
				}

				if _, err := c.readIn.Write(append([]byte(msg.Text), byte('\n'))); err != nil {
					panic(err)
				}
			}
		}()

		// Close Streamer's done channel when we close ours
		c.wg.Add(1)
		go func() {
			<-c.done
			close(streamDoneCh)
			c.wg.Done()
		}()

		// Process outgoing writes to Slack
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			batchCh, errCh := c.Batcher(c.writeOut)

			for batch := range batchCh {
				c.Client.SendMessage(&Message{
					ChannelID: c.SlackChannelID,
					Text:      batch,
				})
			}

			if err := <-errCh; err != nil {
				panic(err)
			}
		}()
	})
}

// Read returns text from the main body of a Slack channel (i.e. excluding
// threads), buffered by line. Single messages will be terminated with an
// appended newline. Messages with explicit line breaks are equivalent to
// multiple single messages in succession.
func (c *ReadWriter) Read(p []byte) (int, error) {
	c.init()
	return c.readOut.Read(p)
}

// Write submits text to the main body of a Slack channel, with message
// boundaries determined by the ReadWriter's Batcher.
func (c *ReadWriter) Write(p []byte) (int, error) {
	c.init()
	return c.writeIn.Write(p)
}

// Close disconnects this ReadWriter from Slack and shuts down internal
// buffers.  After calling Close, the next call to Read will result in an EOF
// and the next call to Write will result in an error.
func (c *ReadWriter) Close() error {
	c.init()

	close(c.done)

	// Close the input of each pipe, so the next Read returns EOF. These are
	// documented to always return nil - this assumption simplifies the
	// implementation of this method.
	c.readIn.Close()
	c.writeIn.Close()

	// If the program is exiting, allow internal goroutines the chance to panic.
	// TODO When we get to the point that multiple slackios are more useful, this
	// entire error strategy should be reconsidered.
	c.wg.Wait()

	return nil
}
