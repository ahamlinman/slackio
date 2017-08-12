package slackio

import (
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

func (r *rtmReader) processMessageEvent(m *slack.MessageEvent) {
	if m.Type != "message" ||
		m.ReplyTo > 0 ||
		m.Channel != r.slackChannel ||
		m.ThreadTimestamp != "" ||
		m.Text == "" {
		return
	}

	if _, err := r.pipeIn.Write(append([]byte(m.Text), byte('\n'))); err != nil {
		panic(err)
	}
}

func (r *rtmReader) Read(p []byte) (int, error) {
	return r.pipeOut.Read(p)
}

func (r *rtmReader) Close() error {
	r.close <- true

	// Close the writer, so the next read receives EOF. In the case of
	// slackbridge the reader is a goroutine in the "os/exec" package, which
	// must terminate before a Cmd's "Wait" method can finish.
	return r.pipeIn.Close()
}
