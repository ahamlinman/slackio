package slackio

import (
	"errors"
	"sync"

	"github.com/nlopes/slack"
)

// messageQueueSize is the maximum size of this Client's message queue. This
// constant balances memory usage with the ability to subscribe at a past point
// in the stream using SubscribeAt. In the future it may be made configurable
// for each Client instance.
const messageQueueSize = 16

// ErrNotSubscribed is returned when an attempt is made to unsubscribe a
// channel that is not currently subscribed.
var ErrNotSubscribed = errors.New("slackio: channel not subscribed")

// ErrSubscriptionOutOfRange is returned when subscribing to a point in the
// stream that no longer exists.
var ErrSubscriptionOutOfRange = errors.New("slackio: subscription out of range")

// Client implements an ability to send and receive Slack messages using a
// real-time API. It provides the underlying functionality for Reader and
// Writer.
//
// A Client instance encapsulates a WebSocket connection to Slack. Users of
// slackio should create a single Client and share it across Reader and Writer
// instances. The connection is made on-demand when a method of the Client is
// first invoked.
type Client struct {
	rtm *slack.RTM

	wg   sync.WaitGroup
	done chan struct{}

	messages      []Message
	messagesLock  sync.RWMutex
	messagesCond  *sync.Cond
	nextMessageID int

	subs     map[chan Message]*subscription
	subsLock sync.Mutex
}

// NewClient returns a new Client that connects to Slack using the given API
// token.
func NewClient(apiToken string) *Client {
	if apiToken == "" {
		panic("slackio: Client requires a non-blank API token")
	}

	c := &Client{}
	c.done = make(chan struct{})
	c.messagesCond = sync.NewCond(c.messagesLock.RLocker())
	c.subs = make(map[chan Message]*subscription)

	api := slack.New(apiToken)
	c.rtm = api.NewRTM()
	go c.rtm.ManageConnection()

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			select {
			case evt := <-c.rtm.IncomingEvents:
				switch data := evt.Data.(type) {
				case *slack.InvalidAuthEvent:
					panic("slackio: Slack API credentials are invalid")

				case *slack.MessageEvent:
					c.distribute(data)
				}

			case <-c.done:
				return
			}
		}
	}()

	return c
}

// distribute pushes non-empty messages from the main body of a Slack channel
// onto the queue for subscriber distribution.
func (c *Client) distribute(m *slack.MessageEvent) {
	if m.Type != "message" ||
		m.ReplyTo > 0 ||
		m.ThreadTimestamp != "" ||
		m.Text == "" {
		return
	}

	c.messagesLock.Lock()
	defer c.messagesLock.Unlock()

	c.messages = append(c.messages, Message{
		ID:        c.nextMessageID,
		ChannelID: m.Channel,
		Text:      m.Text,
	})

	if len(c.messages) > messageQueueSize {
		c.messages = c.messages[1:]
	}

	c.nextMessageID++
	c.messagesCond.Broadcast()
}

// Subscribe adds a channel to this Client. After Subscribe returns, the
// channel will begin receiving an unbounded stream of Messages until the
// channel is unsubscribed. See the SubscribeAt documentation for more details.
func (c *Client) Subscribe(ch chan Message) error {
	return c.SubscribeAt(-1, ch)
}

// SubscribeAt adds a channel to this Client in a manner similar to Subscribe,
// but begins the subscription at a specified point in the message stream. This
// start point is indicated by the monotonically increasing ID contained within
// each Message.
//
// SubscribeAt supports subscriptions arbitrarily far into the future of the
// stream. In this case the subscriber will wait until a message with that ID
// has been received. However, only a fixed number of past messages are kept in
// the stream. If the message with the requested ID is no longer available,
// ErrSubscriptionOutOfRange will be returned. As a special case, a negative ID
// will start the subscription immediately after the most recent message.
func (c *Client) SubscribeAt(id int, ch chan Message) error {
	if id < 0 {
		c.messagesLock.RLock()
		id = c.nextMessageID
		c.messagesLock.RUnlock()
	}

	if len(c.messages) > 0 && id < c.messages[0].ID {
		return ErrSubscriptionOutOfRange
	}

	c.subsLock.Lock()
	defer c.subsLock.Unlock()

	c.subs[ch] = newSubscription(c, id, ch)
	return nil
}

// Unsubscribe removes a subscribed channel from this Client. After Unsubscribe
// returns, the channel will no longer receive any messages and may safely be
// closed. If the given channel was not previously subscribed, ErrNotSubscribed
// will be returned.
func (c *Client) Unsubscribe(ch chan Message) error {
	c.subsLock.Lock()
	defer c.subsLock.Unlock()

	if _, ok := c.subs[ch]; !ok {
		return ErrNotSubscribed
	}

	c.subs[ch].stop()
	delete(c.subs, ch)
	return nil
}

// SendMessage sends the given Message to its associated Slack channel.
func (c *Client) SendMessage(m Message) {
	msg := c.rtm.NewOutgoingMessage(m.Text, m.ChannelID)
	c.rtm.SendMessage(msg)
}

// Close unsubscribes all subscribed channels and disconnects from Slack. The
// behavior of Subscribe, SubscribeAt, and Unsubscribe for a closed Client is
// undefined.
func (c *Client) Close() error {
	close(c.done)
	c.wg.Wait()

	c.subsLock.Lock()
	defer c.subsLock.Unlock()

	for _, sub := range c.subs {
		sub.stop()
	}

	// Subscriptions wait on new messages in a goroutine. This final post-closure
	// Broadcast terminates those goroutines if they exist (see subscription.go).
	c.messagesCond.Broadcast()

	return c.rtm.Disconnect()
}
