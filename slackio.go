package slackio

import (
	"errors"
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
	// TODO There has to be a better way to do this... something like Tyler's
	// phpjson thing.
	err1 := s.reader.Close()
	err2 := s.writer.Close()
	err3 := s.disconnect()
	if err1 != nil || err2 != nil || err3 != nil {
		return errors.New("slackio failed to close")
	}

	return nil
}
