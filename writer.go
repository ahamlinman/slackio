package slackio

import (
	"errors"
	"io"
	"sync"
)

// WriteClient represents objects that can send slackio Messages. Note that in
// slackio, Client implements this interface.
type WriteClient interface {
	SendMessage(Message)
}

// Writer writes messages to the main body of a single Slack channel.
type Writer struct {
	Client         WriteClient // required
	SlackChannelID string      // required
	Batcher        Batcher     // uses DefaultBatcher if not provided

	initOnce sync.Once
	wg       sync.WaitGroup
	writeOut io.ReadCloser
	writeIn  io.WriteCloser
}

// init initializes internal state for a Writer and spawns internal goroutines.
// It must be called at the beginning of all other Writer methods.
func (c *Writer) init() {
	c.initOnce.Do(func() {
		if c.Client == nil || c.SlackChannelID == "" {
			panic(errors.New("slackio: Client and SlackChannelID are required for Writer"))
		}

		if c.Batcher == nil {
			c.Batcher = DefaultBatcher
		}

		c.writeOut, c.writeIn = io.Pipe()

		// Process outgoing writes to Slack
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			batchCh, errCh := c.Batcher(c.writeOut)

			for batch := range batchCh {
				c.Client.SendMessage(Message{
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

// Write submits text to the main body of a Slack channel, with message
// boundaries determined by the Writer's Batcher.
func (c *Writer) Write(p []byte) (int, error) {
	c.init()
	return c.writeIn.Write(p)
}

// Close disconnects this Writer from Slack and shuts down internal buffers.
// After calling Close, the next call to Write will result in an error.
func (c *Writer) Close() error {
	c.init()

	c.writeIn.Close() // Always returns nil
	c.wg.Wait()
	return nil
}
