package slackio

import (
	"errors"
	"io"
	"reflect"
	"sync"
	"testing"
)

func mockStaticBatcher(r io.Reader) (<-chan string, <-chan error) {
	outCh, errCh := make(chan string), make(chan error, 1)

	var readBytes [8]byte

	go func() {
		defer func() {
			close(outCh)
			close(errCh)
		}()

		for {
			_, err := r.Read(readBytes[:])
			if err == io.EOF {
				return
			} else if err != nil {
				errCh <- err
				return
			}

			outCh <- "(batch)"
		}
	}()

	return outCh, errCh
}

func createMockErrBatcher(err error) Batcher {
	return func(_ io.Reader) (<-chan string, <-chan error) {
		outCh, errCh := make(chan string), make(chan error, 1)

		go func() {
			close(outCh)
			errCh <- err
			close(errCh)
		}()

		return outCh, errCh
	}
}

type testWriteClient struct {
	lastMessage Message

	initOnce   sync.Once
	gotMessage chan struct{}
}

func (c *testWriteClient) SendMessage(m Message) {
	c.initOnce.Do(func() { c.gotMessage = make(chan struct{}) })
	c.lastMessage = m
	c.gotMessage <- struct{}{}
}

func (c *testWriteClient) wait() {
	c.initOnce.Do(func() { c.gotMessage = make(chan struct{}) })
	<-c.gotMessage
}

func TestWriter(t *testing.T) {
	client := &testWriteClient{}
	w := NewWriter(client, "C12345678", mockStaticBatcher)

	if _, err := w.Write([]byte("test")); err != nil {
		t.Fatalf("unexpected Writer error: %q", err.Error())
	}

	client.wait()

	expectedMessage := Message{
		ChannelID: "C12345678",
		Text:      "(batch)",
	}

	if !reflect.DeepEqual(client.lastMessage, expectedMessage) {
		t.Fatalf("message %#v did not match expectations (expected %#v)", client.lastMessage, expectedMessage)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("unexpected Writer error on close: %q", err.Error())
	}
}

func TestNewWriterRequiresChannelID(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatal("NewWriter did not panic with no channelID")
		}
	}()

	NewWriter(&testWriteClient{}, "", nil)
}

func TestNewWriterSetsDefaultBatcher(t *testing.T) {
	w := NewWriter(&testWriteClient{}, "C12345678", nil)
	defer w.Close()

	if w.batcher == nil {
		t.Fatal("NewWriter did not set a default batcher")
	}
}

func TestWriterCloseReturnsBatcherError(t *testing.T) {
	berr := errors.New("mock Batcher error")
	batcher := createMockErrBatcher(berr)

	w := NewWriter(&testWriteClient{}, "C12345678", batcher)
	if err := w.Close(); err != berr {
		t.Fatalf("Writer returned unexpected error on Close: %q", err.Error())
	}
}
