package slackio

import (
	"testing"
	"time"

	"github.com/nlopes/slack"
)

func TestBasicSubscription(t *testing.T) {
	msg := slack.Msg{Type: "message", Channel: "C12345678", Text: "hi"}
	evt := slack.MessageEvent(slack.Message{Msg: msg})

	// The first case is that of a totally empty Client that has not received any
	// messages yet. In this case the queue length is 0, and we are forced to
	// wait for a message.

	c := initClient()
	ch := make(chan Message)
	sub := newSubscription(c, 0, ch)

	// Try to help ensure that the subscriber goroutine gets to the point of
	// waiting. This isn't perfect but should at least help.
	time.Sleep(10 * time.Millisecond)
	c.distribute(&evt)
	time.Sleep(10 * time.Millisecond)
	c.distribute(&evt)
	time.Sleep(10 * time.Millisecond)
	c.distribute(&evt) // test bailing early from blocking send

	for i := 0; i < 2; i++ {
		out := <-ch

		if out.ID != i && out.Text != msg.Text {
			t.Fatalf("unexpected message: %#v", out)
		}
	}

	sub.stop()
	c.messagesCond.Broadcast()
}

func TestInPastSubscription(t *testing.T) {
	msg := slack.Msg{Type: "message", Channel: "C12345678", Text: "hi"}
	evt := slack.MessageEvent(slack.Message{Msg: msg})

	// In this case, the Client has already received messages. The subscription
	// should make them available.

	c := initClient()
	c.distribute(&evt)
	c.distribute(&evt)
	c.distribute(&evt)

	ch := make(chan Message)
	sub := newSubscription(c, 0, ch)

	for i := 0; i < 3; i++ {
		out := <-ch

		if out.ID != i && out.Text != msg.Text {
			t.Fatalf("unexpected message: %#v", out)
		}
	}

	// Again, try to hopefully get to the waiting state.
	time.Sleep(10 * time.Millisecond)

	select {
	case <-ch:
		t.Fatal("somehow able to receive a message that wasn't sent")
	default:
	}

	sub.stop()
	c.messagesCond.Broadcast()
}

func TestForwardSkippedSubscription(t *testing.T) {
	msg := slack.Msg{Type: "message", Channel: "C12345678", Text: "hi"}
	evt := slack.MessageEvent(slack.Message{Msg: msg})

	// In this case, the subscription has fallen behind and needs to skip
	// messages to catch up.

	c := initClient()
	c.nextMessageID = 5 // cheating a bit

	c.distribute(&evt)
	c.distribute(&evt)
	c.distribute(&evt)

	ch := make(chan Message)
	sub := newSubscription(c, 0, ch)

	for i := 0; i < 3; i++ {
		out := <-ch

		if out.ID != i+5 && out.Text != msg.Text {
			t.Fatalf("unexpected message: %#v", out)
		}
	}

	sub.stop()
	c.messagesCond.Broadcast()
}
