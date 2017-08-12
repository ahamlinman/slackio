package slackio

import (
	"errors"
	"io"

	"github.com/nlopes/slack"
)

// rtmReader is a ReadCloser that reads from a Slack channel using the
// real-time API. When a new line becomes available from a Slack message, a
// trailing newline is appended and the text becomes available through the Read
// method.
type rtmReader struct {
	rtm          *slack.RTM
	slackChannel string
	close        chan bool
	pipeIn       io.WriteCloser
	pipeOut      io.ReadCloser
}

func newRTMReader(rtm *slack.RTM, slackChannel string) *rtmReader {
	pipeReader, pipeWriter := io.Pipe()

	r := &rtmReader{
		rtm:          rtm,
		slackChannel: slackChannel,
		close:        make(chan bool),
		pipeIn:       pipeWriter,
		pipeOut:      pipeReader,
	}
	go r.processRTMStream()

	return r
}

// processRTMStream runs as a goroutine to continually process Slack messages.
func (r *rtmReader) processRTMStream() {
	for {
		select {
		case evt := <-r.rtm.IncomingEvents:
			if data, ok := evt.Data.(*slack.MessageEvent); ok {
				r.processMessageEvent(data)
			}

		case <-r.close:
			return
		}
	}
}

func (r *rtmReader) processMessageEvent(data *slack.MessageEvent) {
	if data.Type != "message" || data.Channel != r.slackChannel || data.Text == "" {
		return
	}

	if _, err := r.pipeIn.Write(append([]byte(data.Text), byte('\n'))); err != nil {
		// TODO Consider other options for handling this
		panic(err)
	}
}

func (r *rtmReader) Read(p []byte) (int, error) {
	return r.pipeOut.Read(p)
}

func (r *rtmReader) Close() error {
	r.close <- true

	// TODO report this better
	err1 := r.pipeIn.Close()
	err2 := r.pipeOut.Close()
	if err1 != nil || err2 != nil {
		return errors.New("rtmReader failed to close pipes")
	}

	return nil
}
