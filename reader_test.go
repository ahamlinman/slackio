package slackio

import (
	"bytes"
	"io"
	"sync"
	"testing"
)

type testReadClient struct {
	messages []Message
	wg       sync.WaitGroup
}

func (c *testReadClient) GetMessageStream() (<-chan Message, chan<- struct{}) {
	outCh, doneCh := make(chan Message), make(chan struct{})

	c.wg.Add(1)
	go func() {
		for _, m := range c.messages {
			outCh <- m
		}
		close(outCh)
		c.wg.Done()
	}()

	// Note that this mock implementation of GetMessageStream depends on the
	// caller closing the done channel. In general this is not required when the
	// message stream is closed first.
	c.wg.Add(1)
	go func() {
		<-doneCh
		c.wg.Done()
	}()

	return outCh, doneCh
}

func (c *testReadClient) wait() {
	c.wg.Wait()
}

func TestReader(t *testing.T) {
	client := &testReadClient{
		messages: []Message{
			{
				Text:      "a message",
				ChannelID: "C12345678",
			},
			{
				Text:      "and another",
				ChannelID: "C87654321",
			},
		},
	}

	r := &Reader{Client: client}
	var readBytes [16]byte

	expected := [][]byte{[]byte("a message\n"), []byte("and another\n")}

	for _, e := range expected {
		if _, err := r.Read(readBytes[:]); err != nil {
			t.Fatalf("unexpected Reader error: %q", err.Error())
		}

		if !bytes.HasPrefix(readBytes[:], e) {
			t.Fatalf("unexpected Reader output: %q (expected prefix %q)", readBytes, e)
		}
	}

	if err := r.Close(); err != nil {
		t.Fatalf("unexpected Reader error: %q", err.Error())
	}

	client.wait()
	// Test times out if Reader fails to close done channel.
}

func TestSingleChannelReader(t *testing.T) {
	client := &testReadClient{
		messages: []Message{
			{
				Text:      "a message",
				ChannelID: "C12345678",
			},
			{
				Text:      "not this one",
				ChannelID: "C87654321",
			},
			{
				Text:      "and another",
				ChannelID: "C12345678",
			},
		},
	}

	r := &Reader{Client: client, SlackChannelID: "C12345678"}
	var readBytes [16]byte

	expected := [][]byte{[]byte("a message\n"), []byte("and another\n")}

	for _, e := range expected {
		if _, err := r.Read(readBytes[:]); err != nil {
			t.Fatalf("unexpected Reader error: %q", err.Error())
		}

		if !bytes.HasPrefix(readBytes[:], e) {
			t.Fatalf("unexpected Reader output: %q (expected prefix %q)", readBytes, e)
		}
	}

	if err := r.Close(); err != nil {
		t.Fatalf("unexpected Reader error: %q", err.Error())
	}

	client.wait()
	// Test times out if Reader fails to close done channel.
}

func TestReaderDrainsGetMessageStream(t *testing.T) {
	client := &testReadClient{
		messages: []Message{
			{
				Text:      "a message",
				ChannelID: "C12345678",
			},
			{
				Text:      "and another",
				ChannelID: "C87654321",
			},
			{
				Text:      "even more",
				ChannelID: "C08675309",
			},
		},
	}

	r := &Reader{Client: client}
	var readBytes [16]byte

	if _, err := r.Read(readBytes[:]); err != nil {
		t.Fatalf("unexpected Reader error: %q", err.Error())
	}

	// Notice that subsequent messages are not read prior to closure.

	if err := r.Close(); err != nil {
		t.Fatalf("unexpected Reader error: %q", err.Error())
	}

	if _, err := r.Read(readBytes[:]); err != io.EOF {
		t.Fatalf("unexpected Reader error: %q (expected EOF)", err.Error())
	}

	client.wait()
	// Test times out if Reader fails to close done channel.
}

func TestReaderRequiresClient(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatal("Reader did not panic with no Client")
		}
	}()

	r := &Reader{}
	var readBytes [1]byte
	r.Read(readBytes[:])
}
