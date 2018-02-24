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

// ErrAlreadySubscribed is returned when an attempt is made to subscribe a
// channel that already has a subscription.
var ErrAlreadySubscribed = errors.New("slackio: channel already subscribed")

// ErrNotSubscribed is returned when an attempt is made to unsubscribe a
// channel that is not currently subscribed.
var ErrNotSubscribed = errors.New("slackio: channel not subscribed")

// Client implements an ability to send and receive Slack messages using a
// real-time API. For readers, it presents a long-running stream of a user's
// incoming Slack messages that may be consumed using multiple independent
// channels. For writers, it allows instant sending of a message to a given
// channel.
//
// A Client instance encapsulates a WebSocket connection to Slack. Users of
// slackio should create a single Client and share it across Reader and Writer
// instances.
type Client struct {
	rtm *slack.RTM

	wg   sync.WaitGroup
	done chan struct{}

	messages      []Message
	messagesLock  sync.RWMutex
	messagesCond  *sync.Cond
	nextMessageID int

	subs     map[chan<- Message]*subscription
	subsLock sync.Mutex
}

// NewClient returns a new Client and connects it to Slack using the given API
// token. Invalid API tokens will result in a panic while attempting to
// establish the connection.
func NewClient(apiToken string) *Client {
	if apiToken == "" {
		panic("slackio: Client requires a non-blank API token")
	}

	c := initClient()

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
					panic(errors.New("slackio: Slack API credentials are invalid"))

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

// initClient returns a Client with basic fields initialized. It mainly helps
// remove a bit of boilerplate from tests.
func initClient() *Client {
	c := &Client{}

	c.done = make(chan struct{})
	c.messagesCond = sync.NewCond(c.messagesLock.RLocker())
	c.subs = make(map[chan<- Message]*subscription)

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

// Subscribe creates a new subscription for the given channel within this
// Client, starting immediately after the latest message in the client's
// overall message stream. See the SubscribeAt documentation for more details.
func (c *Client) Subscribe(ch chan<- Message) error {
	return c.SubscribeAt(-1, ch)
}

// SubscribeAt creates a new subscription for the given channel within this
// Client, starting at the specified point in the client's overall message
// stream. Each message sent into the channel will have an ID indicating its
// relative position in the stream. IDs begin at 0 and increment by 1 for each
// new message, and are unique within a single Client instance.
//
// Each Client maintains a bounded number of past messages from the overall
// stream. If a subscriber falls behind this buffer, or is subscribed using an
// ID that is no longer in the buffer, that subscriber will transparently be
// skipped forward to the earliest message still remaining in the buffer. All
// intervening messages will be lost. If necessary, subscribers can detect this
// behavior by watching for message ID increases larger than 1.
//
// Subscriptions using IDs that have not yet appeared in the stream are
// supported. The subscription will begin once a new message has been assigned
// the requested ID and inserted into the stream.
//
// As a special case, a negative ID will begin the subscription immediately
// after the latest message in the stream.
//
// If the given channel already has an active subscription,
// ErrAlreadySubscribed will be returned.
func (c *Client) SubscribeAt(id int, ch chan<- Message) error {
	if id < 0 {
		c.messagesLock.RLock()
		id = c.nextMessageID
		c.messagesLock.RUnlock()
	}

	c.subsLock.Lock()
	defer c.subsLock.Unlock()

	if _, ok := c.subs[ch]; ok {
		return ErrAlreadySubscribed
	}

	c.subs[ch] = newSubscription(c, id, ch)
	return nil
}

// Unsubscribe terminates the subscription for the given channel within this
// Client. After Unsubscribe returns, the channel will no longer receive any
// messages and may safely be closed. If the given channel was not previously
// subscribed, ErrNotSubscribed will be returned.
func (c *Client) Unsubscribe(ch chan<- Message) error {
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

// Close terminates all subscriptions within this Client and disconnects from
// Slack. The behavior of Subscribe, SubscribeAt, and Unsubscribe for a closed
// Client is undefined.
func (c *Client) Close() error {
	close(c.done)
	c.wg.Wait()

	c.subsLock.Lock()
	defer c.subsLock.Unlock()

	for _, sub := range c.subs {
		sub.stop()
	}

	// Unblock any subscribers waiting for a new message and allow them to
	// terminate.
	c.messagesCond.Broadcast()

	// Allow for unit testing of the above subscription-related logic.
	if c.rtm != nil {
		return c.rtm.Disconnect()
	}

	return nil
}
