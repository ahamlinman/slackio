package slackio

import (
	"sync"
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
			c := &Client{}
			c.messagesCond = sync.NewCond(c.messagesLock.RLocker())

			// Yes, there are three layers of types here
			evt := slack.MessageEvent(slack.Message{Msg: tc.event})
			c.distribute(&evt)

			if tc.shouldSend {
				if len(c.messages) < 1 {
					t.Fatalf("did not send message when it should have: %#v", tc.event)
				}

				expected := Message{
					ID:        0,
					ChannelID: tc.event.Channel,
					Text:      tc.event.Text,
				}

				if c.messages[0] != expected {
					t.Fatalf("unexpected message %#v (expected %#v)", c.messages[0], expected)
				}
			} else {
				if len(c.messages) > 0 {
					t.Fatalf("sent message when it should not have: %#v", tc.event)
				}
			}
		})
	}
}
