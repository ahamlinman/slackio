// Package slackio implements real-time communication with a single Slack
// channel behind an io.ReadWriteCloser interface.
package slackio

import (
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
	close        chan bool

	readIn  io.WriteCloser
	readOut io.ReadCloser

	writer io.WriteCloser
}

// New returns a Client for the given channel that uses the given Slack API
// token. Note that channel must be an ID rather than a channel name (this can
// be obtained from the URL path when viewing Slack on the web).
func New(token, channel string) *Client {
	api := slack.New(token)
	rtm := api.NewRTM()
	go rtm.ManageConnection()

	readOut, readIn := io.Pipe()

	c := &Client{
		rtm:          rtm,
		slackChannel: channel,
		close:        make(chan bool),

		readIn:  readIn,
		readOut: readOut,

		writer: newRTMWriter(rtm, channel),
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

			case <-c.close:
				return
			}
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
func (s *Client) Read(p []byte) (int, error) {
	return s.readOut.Read(p)
}

// Write submits one or more newline-delimited messages to the main body of a
// Slack channel. Every line is sent as an individual message, and no batching
// of any kind is performed (though this is under consideration as a potential
// improvement).
func (s *Client) Write(p []byte) (int, error) {
	return s.writer.Write(p)
}

// Close disconnects this Client from the real-time API and shuts down internal
// buffers. After calling Close, the next call to Read will result in an EOF
// and the next call to Write will result in an error.
func (s *Client) Close() error {
	s.close <- true

	// These always return nil. See their respective comments.
	s.readIn.Close()
	s.writer.Close()

	return s.rtm.Disconnect()
}
