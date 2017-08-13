// Package slackio implements real-time communication with a single Slack
// channel behind an io.ReadWriteCloser interface.
package slackio

import (
	"io"
	"sync"

	"github.com/nlopes/slack"
)

// Client is a ReadWriteCloser for a single Slack channel, where the content of
// the channel's main body is represented as lines of text.
type Client struct {
	rtm          *slack.RTM
	slackChannel string
	done         chan struct{}
	wg           *sync.WaitGroup

	readIn  io.WriteCloser
	readOut io.ReadCloser

	writeIn  io.WriteCloser
	writeOut io.ReadCloser
}

// New returns a Client that connects to Slack in real-time, using the provided
// API token and filtering by the provided channel ID.
func New(token, channel string) *Client {
	api := slack.New(token)
	rtm := api.NewRTM()
	go rtm.ManageConnection()

	readOut, readIn := io.Pipe()
	writeOut, writeIn := io.Pipe()

	c := &Client{
		rtm:          rtm,
		slackChannel: channel,
		done:         make(chan struct{}),
		wg:           &sync.WaitGroup{},

		readIn:  readIn,
		readOut: readOut,

		writeIn:  writeIn,
		writeOut: writeOut,
	}
	c.init()

	return c
}

// init spawns internal goroutines that manage input and output.
func (c *Client) init() {
	// Process incoming reads
	c.wg.Add(1)
	go func() {
		for {
			select {
			case evt := <-c.rtm.IncomingEvents:
				if data, ok := evt.Data.(*slack.MessageEvent); ok {
					c.processIncomingMessage(data)
				}

			case <-c.done:
				c.wg.Done()
				return
			}
		}
	}()

	// Process outgoing writes
	c.wg.Add(1)
	go func() {
		batchCh, errCh := LineBatcher(c.writeOut)

		for batch := range batchCh {
			msg := c.rtm.NewOutgoingMessage(batch, c.slackChannel)
			c.rtm.SendMessage(msg)
		}

		if err := <-errCh; err != nil {
			panic(err)
		}

		c.wg.Done()
	}()
}

// processIncomingMessage filters Slack messages and extracts their text.
func (c *Client) processIncomingMessage(m *slack.MessageEvent) {
	// On the ReplyTo field: A message with this field is sent by Slack's RTM API
	// when it thinks you have a flaky network connection. It's a copy of *your*
	// previous message, so you can verify that it was sent properly.
	if m.Type != "message" ||
		m.ReplyTo > 0 ||
		m.Channel != c.slackChannel ||
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
	return c.readOut.Read(p)
}

// Write submits one or more newline-delimited messages to the main body of a
// Slack channel. Every line is sent as an individual message.
func (c *Client) Write(p []byte) (int, error) {
	return c.writeIn.Write(p)
}

// Close disconnects this Client from Slack and shuts down internal buffers.
// After calling Close, the next call to Read will result in an EOF and the
// next call to Write will result in an error.
func (c *Client) Close() error {
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
