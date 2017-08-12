package slackio

import (
	"io"

	"github.com/nlopes/slack"
)

// Client is a ReadWriteCloser that reads from and writes to a single Slack
// channel using the Slack real-time API. In this model, the content of the
// main channel is represented as lines of text. Concepts like threads,
// reactions, and even the senders of individual messages are entirely ignored.
type Client struct {
	reader     io.ReadCloser
	writer     io.WriteCloser
	disconnect func() error
}

// New returns a SlackIO for the given channel that uses the given Slack API
// token. channel must be an ID rather than a channel name. This ID can be
// obtained from the URL path when viewing Slack on the web.
func New(token, channel string) *Client {
	api := slack.New(token)
	rtm := api.NewRTM()
	go rtm.ManageConnection()

	return &Client{
		reader:     newRTMReader(rtm, channel),
		writer:     newRTMWriter(rtm, channel),
		disconnect: rtm.Disconnect,
	}
}

// Read returns text from the main body of a Slack channel (i.e. excluding
// threads), buffered by line. Single messages will be terminated with an
// appended newline. Messages with explicit line breaks are equivalent to
// multiple single messages in succession.
func (s *Client) Read(p []byte) (int, error) {
	return s.reader.Read(p)
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
	// These always return nil. See their respective comments.
	s.reader.Close()
	s.writer.Close()

	return s.disconnect()
}
