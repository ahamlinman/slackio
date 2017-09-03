package slackio

import (
	"reflect"
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

func TestSubscribe(t *testing.T) {
	c := &Client{}

	ch1, ch2 := make(chan Message), make(chan Message)
	done := make(chan struct{})

	subscribe := func(ch chan Message) {
		c.Subscribe(ch)
		done <- struct{}{}
	}

	// Take out all locks, run with -race, and watch this fail:
	go subscribe(ch1)
	go subscribe(ch2)
	<-done
	<-done

	if len(c.chanPool) != 2 ||
		(!reflect.DeepEqual(c.chanPool, []chan Message{ch1, ch2}) &&
			!reflect.DeepEqual(c.chanPool, []chan Message{ch2, ch1})) {
		t.Fatalf("unexpected channel pool after Subscribe: %#v", c.chanPool)
	}
}

func TestUnsubscribe(t *testing.T) {
	c := &Client{}

	ch1, ch2 := make(chan Message), make(chan Message)
	done := make(chan struct{})
	c.chanPool = append(c.chanPool, ch1, ch2)

	unsubscribe := func(ch chan Message) {
		if err := c.Unsubscribe(ch); err != nil {
			t.Fatalf("unexpected unsubscribe error: %s", err.Error())
		}

		done <- struct{}{}
	}

	// Take out all locks, run with -race, and watch this fail:
	go unsubscribe(ch1)
	go unsubscribe(ch2)
	<-done
	<-done

	if err := c.Unsubscribe(ch1); err != ErrNotSubscribed {
		t.Fatalf("unexpected unsubscribe error: %#v (expected ErrNotSubscribed)", err)
	}
}
