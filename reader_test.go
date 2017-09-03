package slackio

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"
)

type testReadClient struct {
	messages  []Message
	wg        sync.WaitGroup
	doneChans map[chan Message]chan struct{}
}

// Subscribe in this test implementation just sends a predefined set of
// messages into a channel.
func (c *testReadClient) Subscribe(ch chan Message) error {
	if c.doneChans == nil {
		c.doneChans = make(map[chan Message]chan struct{})
	}

	done := make(chan struct{})
	c.doneChans[ch] = done

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer close(done)

		for _, m := range c.messages {
			ch <- m
		}
	}()

	return nil
}

// Unsubscribe in this test implementation blocks until Subscribe is done
// sending messages. This is strictly valid, and helps ensure that Reader fully
// drains the channel until the unsubscription is complete.
func (c *testReadClient) Unsubscribe(ch chan Message) error {
	done := c.doneChans[ch]
	if done == nil {
		return errors.New("channel not subscribed")
	}

	<-done
	delete(c.doneChans, ch)
	return nil
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
	// Test times out if Reader fails to stop properly
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
	// Test times out if Reader fails to stop properly
}

func TestReaderDrainsSubscribedChannel(t *testing.T) {
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

	// Notice that subsequent messages are not read prior to closure. Reader must
	// drain these properly, or client.wait() will time out forever.

	if err := r.Close(); err != nil {
		t.Fatalf("unexpected Reader error: %q", err.Error())
	}

	if _, err := r.Read(readBytes[:]); err != io.EOF {
		t.Fatalf("unexpected Reader error: %q (expected EOF)", err.Error())
	}

	client.wait()
	// Test times out if Reader fails to stop properly
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
