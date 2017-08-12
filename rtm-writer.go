package slackio

import (
	"bufio"
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

// processWrites runs as a goroutine to continually process written data.
func (w *rtmWriter) processWrites() {
	scanner := bufio.NewScanner(w.pipeOut) // Breaks on newlines by default
	for scanner.Scan() {
		msg := w.rtm.NewOutgoingMessage(scanner.Text(), w.slackChannel)
		w.rtm.SendMessage(msg)
	}

	if err := scanner.Err(); err != nil {
		panic(err)
	}
}

func (w *rtmWriter) Close() error {
	// Close the writer, so the scanner receives EOF and its goroutine
	// terminates. This method is defined to always return nil.
	w.pipeIn.Close()

	return nil
}
