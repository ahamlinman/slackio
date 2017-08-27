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
}
