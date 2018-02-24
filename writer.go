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
	client    WriteClient
	channelID string
	batcher   Batcher
	wg        sync.WaitGroup
	writeOut  io.ReadCloser
	writeIn   io.WriteCloser
	writeErr  error
}

// NewWriter returns a new Writer. channelID must be non-blank, or NewWriter
// will panic. If batcher is nil, DefaultBatcher will be used as the Batcher.
func NewWriter(client WriteClient, channelID string, batcher Batcher) *Writer {
	if channelID == "" {
		panic(errors.New("slackio: Writer's channelID cannot be blank"))
	}

	if batcher == nil {
		batcher = DefaultBatcher
	}

	c := &Writer{
		client:    client,
		channelID: channelID,
		batcher:   batcher,
	}

	c.writeOut, c.writeIn = io.Pipe()

	// Process outgoing writes to Slack
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		batchCh, errCh := c.batcher(c.writeOut)

		for batch := range batchCh {
			c.client.SendMessage(Message{
				ChannelID: c.channelID,
				Text:      batch,
			})
		}

		c.writeErr = <-errCh
	}()

	return c
}

// Write submits text to the main body of a Slack channel, with message
// boundaries determined by the Writer's Batcher.
func (c *Writer) Write(p []byte) (int, error) {
	return c.writeIn.Write(p)
}

// Close disconnects this Writer from Slack and shuts down internal buffers.
// After calling Close, the next call to Write will result in an error.
func (c *Writer) Close() error {
	c.writeIn.Close() // Always returns nil
	c.wg.Wait()
	return c.writeErr
}
