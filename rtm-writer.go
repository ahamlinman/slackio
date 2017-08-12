package slackio

import (
	"bufio"
	"errors"
	"io"

	"github.com/nlopes/slack"
)

// rtmWriter is a WriteCloser that writes to a Slack channel using the
// real-time API. Once a full line has been written through the Write method,
// it will strip the newline and send the text as a message.
type rtmWriter struct {
	rtm          *slack.RTM
	slackChannel string
	pipeIn       io.WriteCloser
	pipeOut      io.ReadCloser
}

func newRTMWriter(rtm *slack.RTM, slackChannel string) *rtmWriter {
	pipeReader, pipeWriter := io.Pipe()

	w := &rtmWriter{
		rtm:          rtm,
		slackChannel: slackChannel,
		pipeIn:       pipeWriter,
		pipeOut:      pipeReader,
	}
	go w.processWrites()

	return w
}

func (w *rtmWriter) Write(p []byte) (int, error) {
	return w.pipeIn.Write(p)
}

// processWrites runs as a goroutine to continually process newly-written data
// and break it on newlines.
func (w *rtmWriter) processWrites() {
	// TODO This only writes after newline termination. Would it be better to
	// write non-terminated input with some sort of debounce interval?
	scanner := bufio.NewScanner(w.pipeOut)
	for scanner.Scan() {
		msg := w.rtm.NewOutgoingMessage(scanner.Text(), w.slackChannel)
		w.rtm.SendMessage(msg)
	}
}

func (w *rtmWriter) Close() error {
	// TODO report this better
	err1 := w.pipeIn.Close()
	err2 := w.pipeOut.Close()
	if err1 != nil || err2 != nil {
		return errors.New("rtmWriter failed to close pipes")
	}

	return nil
}
