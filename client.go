// Package slackio implements real-time communication with a single Slack
// channel behind an io.ReadWriteCloser interface.
package slackio

import (
	"errors"
	"io"
	"sync"

	"github.com/nlopes/slack"
)

// Client is a ReadWriteCloser for a single Slack channel, where the content of
// the channel's main body is represented as lines of text.
type Client struct {
	APIToken       string // required
	SlackChannelID string // required
	Batcher        Batcher

	initOnce sync.Once
	wg       sync.WaitGroup
	rtm      *slack.RTM
	done     chan struct{}
	readOut  io.ReadCloser
	readIn   io.WriteCloser
	writeOut io.ReadCloser
	writeIn  io.WriteCloser
}

// init initializes internal state for a Client and spawns internal goroutines.
// It must be called at the beginning of all other Client methods.
func (c *Client) init() {
	c.initOnce.Do(func() {
		if c.APIToken == "" || c.SlackChannelID == "" {
			panic(errors.New("APIToken and SlackChannel are required"))
		}

		if c.Batcher == nil {
			c.Batcher = DefaultBatcher
		}

		api := slack.New(c.APIToken)
		c.rtm = api.NewRTM()
		go c.rtm.ManageConnection()

		c.done = make(chan struct{})
		c.readOut, c.readIn = io.Pipe()
		c.writeOut, c.writeIn = io.Pipe()

		// Process incoming reads
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			for {
				select {
				case evt := <-c.rtm.IncomingEvents:
					if data, ok := evt.Data.(*slack.MessageEvent); ok {
						c.processIncomingMessage(data)
					}

				case <-c.done:
					return
				}
			}
		}()

		// Process outgoing writes
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			batchCh, errCh := c.Batcher(c.writeOut)

			for batch := range batchCh {
				msg := c.rtm.NewOutgoingMessage(batch, c.SlackChannelID)
				c.rtm.SendMessage(msg)
			}

			if err := <-errCh; err != nil {
				panic(err)
			}
		}()
	})
}

// processIncomingMessage filters Slack messages and extracts their text.
func (c *Client) processIncomingMessage(m *slack.MessageEvent) {
	// On the ReplyTo field: A message with this field is sent by Slack's RTM API
	// when it thinks you have a flaky network connection. It's a copy of *your*
	// previous message, so you can verify that it was sent properly.
	if m.Type != "message" ||
		m.ReplyTo > 0 ||
		m.Channel != c.SlackChannelID ||
		m.ThreadTimestamp != "" ||
		m.Text == "" {
		return
	}

	if _, err := c.readIn.Write(append([]byte(m.Text), byte('\n'))); err != nil {
		panic(err)
	}
}

// Read returns text from the main body of a Slack channel (i.e. excluding
// threads), buffered by line. Single messages will be terminated with an
// appended newline. Messages with explicit line breaks are equivalent to
// multiple single messages in succession.
func (c *Client) Read(p []byte) (int, error) {
	c.init()
	return c.readOut.Read(p)
}

// Write submits text to the main body of a Slack channel, with message
// boundaries determined by the DefaultBatcher.
func (c *Client) Write(p []byte) (int, error) {
	c.init()
	return c.writeIn.Write(p)
}

// Close disconnects this Client from Slack and shuts down internal buffers.
// After calling Close, the next call to Read will result in an EOF and the
// next call to Write will result in an error.
func (c *Client) Close() error {
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

	return c.rtm.Disconnect()
}
