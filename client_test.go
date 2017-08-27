package slackio

import (
	"testing"

	"github.com/nlopes/slack"
)

func TestDistribute(t *testing.T) {
	cases := []struct {
		description string
		event       slack.Msg
		shouldSend  bool
	}{
		{
			description: "does not send non-messages",
			event:       slack.Msg{Type: "not_message"},
			shouldSend:  false,
		},
		{
			description: "does not send replyTo messages",
			event:       slack.Msg{Type: "message", ReplyTo: 2},
			shouldSend:  false,
		},
		{
			description: "does not send thread messages",
			event:       slack.Msg{Type: "message", ThreadTimestamp: "1234.5678"},
			shouldSend:  false,
		},
		{
			description: "does not send blank messages",
			event:       slack.Msg{Type: "message"},
			shouldSend:  false,
		},
		{
			description: "sends other messages to all channels",
			event:       slack.Msg{Type: "message", Channel: "C12345678", Text: "hi"},
			shouldSend:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			ch1, ch2 := make(chan Message, 1), make(chan Message, 1)

			c := Client{chanPool: []chan Message{ch1, ch2}}
			c.initOnce.Do(func() {})

			// Yes, there are three layers of types here
			evt := slack.MessageEvent(slack.Message{Msg: tc.event})
			c.distribute(&evt)

			if tc.shouldSend {
				var m1, m2 Message

				for i := 0; i < 2; i++ {
					select {
					case m1 = <-ch1:
						continue

					case m2 = <-ch2:
						continue

					default:
						t.Fatal("failed to receive message on all channels")
					}
				}

				if m1 != m2 {
					t.Error("did not receive equivalent message values")
				}
			} else {
				select {
				case <-ch1:
				case <-ch2:
					t.Fatal("sent message when it should not have")

				default:
					return
				}
			}
		})
	}
}

func TestGetMessageStream(t *testing.T) {
	c := &Client{}
	c.initOnce.Do(func() {})
	c.done = make(chan struct{})

	var msgChans [3]<-chan Message
	var doneChans [3]chan<- struct{}

	for i := range msgChans {
		msgChans[i], doneChans[i] = c.GetMessageStream()

		if c.chanPool[i] != msgChans[i] {
			t.Fatal("channel was not added to channel pool")
		}
	}

	close(doneChans[1])
	<-msgChans[1]

	if len(c.chanPool) != 2 ||
		c.chanPool[0] != msgChans[0] ||
		c.chanPool[1] != msgChans[2] {
		t.Fatal("initial channel pool removal was incorrect")
	}

	close(doneChans[0])
	close(doneChans[2])
	<-msgChans[0]
	<-msgChans[2]

	if len(c.chanPool) != 0 {
		t.Fatal("final channel pool removal was incorrect")
	}

	if err := c.Close(); err != nil {
		t.Fatalf("unexpected Client error on Close: %q", err.Error())
	}
}

func TestClientClose(t *testing.T) {
	c := &Client{}
	c.initOnce.Do(func() {})
	c.done = make(chan struct{})

	var msgChans [50]<-chan Message
	var doneChans [50]chan<- struct{}

	for i := range msgChans {
		msgChans[i], doneChans[i] = c.GetMessageStream()
	}

	// Some channels might be closed before Close is called; we should account
	// for this.
	close(doneChans[0])
	<-msgChans[0]

	closuresDone := make(chan struct{})
	go func() {
		for i := range msgChans {
			if i == 0 {
				continue // see above
			}

			<-msgChans[i]

			// Consumers *might* close their done channel even when upstream closes;
			// we should account for this.
			if i == 1 {
				close(doneChans[i])
			}
		}

		close(closuresDone)
	}()

	if err := c.Close(); err != nil {
		t.Fatalf("unexpected Client error on Close: %q", err.Error())
	}

	if len(c.chanPool) != 0 {
		t.Fatal("Close did not block until all channels were removed from pool")
	}

	<-closuresDone
	// If we somehow manage to not close a message stream, the test will time
	// out here.
}
