// Package slackio implements real-time communication with a single Slack
// channel behind an io.ReadWriteCloser interface.
package slackio

import (
	"bufio"
	"io"

	"github.com/nlopes/slack"
)

// Client is a ReadWriteCloser for a single Slack channel, backed by the Slack
// real-time API. The content of the channel's main body is represented as
// lines of text. Concepts like users, threads, and reactions are entirely
// ignored.
type Client struct {
	rtm          *slack.RTM
	slackChannel string
	done         chan struct{}

	readIn  io.WriteCloser
	readOut io.ReadCloser

	writeIn  io.WriteCloser
	writeOut io.ReadCloser
}

// New returns a Client for the given channel that uses the given Slack API
// token. Note that channel must be an ID rather than a channel name (this can
// be obtained from the URL path when viewing Slack on the web).
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
	go func() {
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
	go func() {
		scanner := bufio.NewScanner(c.writeOut) // Breaks on newlines by default
		for scanner.Scan() {
			msg := c.rtm.NewOutgoingMessage(scanner.Text(), c.slackChannel)
			c.rtm.SendMessage(msg)
		}

		if err := scanner.Err(); err != nil {
			panic(err)
		}
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
// Slack channel. Every line is sent as an individual message, and no batching
// of any kind is performed (though this is under consideration as a potential
// improvement).
func (c *Client) Write(p []byte) (int, error) {
	return c.writeIn.Write(p)
}

// Close disconnects this Client from the real-time API and shuts down internal
// buffers. After calling Close, the next call to Read will result in an EOF
// and the next call to Write will result in an error.
func (c *Client) Close() error {
	c.done <- struct{}{}

	// Close the input of each pipe, so the next Read returns EOF. These are
	// documented to always return nil - this assumption simplifies the
	// implementation of this method.
	c.readIn.Close()
	c.writeIn.Close()

	return c.rtm.Disconnect()
}
