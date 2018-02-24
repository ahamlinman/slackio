package slackio

import (
	"sync"
	"testing"

	"github.com/nlopes/slack"
)

func TestNewClientPanicsWithBlankToken(t *testing.T) {
	defer func() {
		if err := recover(); err != "slackio: Client requires a non-blank API token" {
			t.Fatalf("unexpected NewClient error on blank token: %v", err)
		}
	}()

	NewClient("")
}

func TestDistributeFiltering(t *testing.T) {
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
			c := initClient()

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

func TestDistributeRollover(t *testing.T) {
	msg := slack.Msg{Type: "message", Channel: "C12345678", Text: "hi"}
	evt := slack.MessageEvent(slack.Message{Msg: msg})

	c := initClient()
	for i := 0; i < messageQueueSize+1; i++ {
		c.distribute(&evt)
	}

	if len(c.messages) != messageQueueSize {
		t.Errorf("unexpected message queue size %d (expected %d)", len(c.messages), messageQueueSize)
	}

	if c.messages[0].ID != 1 {
		t.Errorf("unexpected message ID at start of queue: %d (expected 1)", c.messages[0].ID)
	}
}

func TestSubscriptionOperations(t *testing.T) {
	// Yes, this test rolls up SubscribeAt, Subscribe, and Unsubscribe all into
	// one case. This is done because the operations are all so interrelated, and
	// it's faster for me to write. Might be worth refactoring later.

	c := initClient()
	ch1, ch2 := make(chan Message), make(chan Message)
	var subWait sync.WaitGroup

	// Notice that multiple subscription operations can run concurrently. This
	// helps the race detector catch errors.

	subWait.Add(1)
	go func() {
		defer subWait.Done()

		if err := c.SubscribeAt(2, ch1); err != nil {
			t.Fatalf("unexpected error on valid subscription: %s", err.Error())
		}

		if err := c.SubscribeAt(3, ch1); err != ErrAlreadySubscribed {
			t.Fatalf("unexpected error on duplicate subscription: %v", err)
		}
	}()

	subWait.Add(1)
	go func() {
		defer subWait.Done()

		if err := c.Subscribe(ch2); err != nil {
			t.Fatalf("unexpected error on valid subscription: %s", err.Error())
		}

		if err := c.Subscribe(ch2); err != ErrAlreadySubscribed {
			t.Fatalf("unexpected error on duplicate subscription: %v", err)
		}

		// Be careful, we need to sync with the above goroutine!
		c.subsLock.Lock()
		defer c.subsLock.Unlock()

		// Not going to claim this is clean, but I want to validate it somehow...
		if c.subs[ch2].id != c.nextMessageID {
			t.Fatalf("unexpected default subscription ID %d", c.subs[ch2].id)
		}
	}()

	subWait.Wait()

	if len(c.subs) != 2 {
		t.Fatalf("unexpected subscription pool length %d (expected 2)", len(c.subs))
	}

	var unsubWait sync.WaitGroup
	unsub := func(ch chan Message) {
		defer unsubWait.Done()

		if err := c.Unsubscribe(ch); err != nil {
			t.Fatalf("unexpected unsubscribe error: %s", err.Error())
		}
	}

	// Again, notice the concurrency.

	unsubWait.Add(2)
	go unsub(ch1)
	go unsub(ch2)
	unsubWait.Wait()

	if err := c.Unsubscribe(ch1); err != ErrNotSubscribed {
		t.Fatalf("unexpected duplicate unsubscribe error: %v", err)
	}

	if len(c.subs) != 0 {
		t.Fatalf("unexpected subscription pool length %d (expected 0)", len(c.subs))
	}
}

func TestClientClose(t *testing.T) {
	c := initClient()
	n := 3

	chans := make([]chan Message, n)
	for i := 0; i < n; i++ {
		chans[i] = make(chan Message)
		c.Subscribe(chans[i])
	}

	i := 0
	subs := make([]*subscription, n)
	for _, sub := range c.subs {
		subs[i] = sub
		i++
	}

	// This helps us test that the final unblocking Broadcast call gets made.
	finalBroadcastCh := make(chan struct{})
	go func() {
		c.messagesLock.RLock()
		finalBroadcastCh <- struct{}{}
		c.messagesCond.Wait()
		c.messagesLock.RUnlock()
		close(finalBroadcastCh)
	}()

	// Guarantee that the goroutine above is blocked in the Wait call. We can't
	// get the write lock until Wait forces release of the read lock.
	<-finalBroadcastCh
	c.messagesLock.Lock()
	c.messagesLock.Unlock()

	// Here we go...
	if err := c.Close(); err != nil {
		t.Fatalf("unexpected Close error: %s", err.Error())
	}

	for _, sub := range subs {
		if sub.active() {
			t.Fatal("subscription reported itself active after Close")
		}
	}

	// If the final Broadcast isn't performed, this will time out.
	<-finalBroadcastCh
}
