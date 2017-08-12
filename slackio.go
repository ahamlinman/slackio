package slackio

import (
	"io"

	"github.com/nlopes/slack"
)

// slackIO is a ReadWriteCloser that reads from and writes to a Slack channel,
// using the Slack real-time API. Reads and writes occur by line.
type slackIO struct {
	reader     io.ReadCloser
	writer     io.WriteCloser
	disconnect func() error
}

func New(token, channel string) *slackIO {
	api := slack.New(token)
	rtm := api.NewRTM()
	go rtm.ManageConnection()

	return &slackIO{
		reader:     newRTMReader(rtm, channel),
		writer:     newRTMWriter(rtm, channel),
		disconnect: rtm.Disconnect,
	}
}

func (s *slackIO) Read(p []byte) (int, error) {
	return s.reader.Read(p)
}

func (s *slackIO) Write(p []byte) (int, error) {
	return s.writer.Write(p)
}

func (s *slackIO) Close() error {
	require := func(f func() error) {
		if err := f(); err != nil {
			panic(err)
		}
	}

	require(s.reader.Close)
	require(s.writer.Close)

	if err := s.disconnect(); err != nil {
		return err
	}

	return nil
}
