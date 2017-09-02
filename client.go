package slackio

import (
	"errors"
	"sync"

	"github.com/nlopes/slack"
)

// ErrNotSubscribed is returned when an attempt is made to unsubscribe a
// channel that is not currently subscribed.
var ErrNotSubscribed = errors.New("slackio: channel not subscribed")

// Client implements an ability to send and receive Slack messages using a
// real-time API. It provides the underlying functionality for Reader and
// Writer.
//
// A Client instance encapsulates a WebSocket connection to Slack. Users of
// slackio should create a single Client and share it across Reader and Writer
// instances. The connection is made on-demand when a method of the Client is
// first invoked.
type Client struct {
	APIToken string // required

	rtm        *slack.RTM
	initOnce   sync.Once
	wg         sync.WaitGroup
	done       chan struct{}
	chanPool   []chan Message
	chanPoolMu sync.Mutex
}

// init sets up state and spawns internal goroutines for a Client. It must be
// called at the start of every exported method.
func (c *Client) init() {
	c.initOnce.Do(func() {
		if c.APIToken == "" {
			panic("slackio: Client requires APIToken")
		}

		c.done = make(chan struct{})

		api := slack.New(c.APIToken)
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
	})
}

// distribute fans out a message to the channels that have been created using
// GetMessageStream.
func (c *Client) distribute(m *slack.MessageEvent) {
	if m.Type != "message" ||
		m.ReplyTo > 0 ||
		m.ThreadTimestamp != "" ||
		m.Text == "" {
		return
	}

	c.chanPoolMu.Lock()
	defer c.chanPoolMu.Unlock()

	for _, ch := range c.chanPool {
		ch <- Message{ChannelID: m.Channel, Text: m.Text}
	}
}

// Subscribe adds a channel to this Client. After Subscribe returns, the
// channel will begin receiving an unbounded stream of Messages until the
// channel is unsubscribed.
//
// It is not safe to call Subscribe synchronously in a loop that processes
// messages from a subscribed channel. A send into the subscribed channel will
// block and keep Subscribe from obtaining an internal lock, resulting in a
// deadlock.
func (c *Client) Subscribe(msgs chan Message) error {
	c.init()

	c.chanPoolMu.Lock()
	defer c.chanPoolMu.Unlock()

	c.chanPool = append(c.chanPool, msgs)
	return nil
}

// Unsubscribe removes a subscribed channel from this Client. After Unsubscribe
// returns, the channel will no longer receive any messages and may safely be
// closed. If the given channel was not previously subscribed, ErrNotSubscribed
// will be returned.
//
// It is not safe to call Unsubscribe synchronously in a loop that processes
// messages from a subscribed channel. A send into the subscribed channel will
// block and keep Unsubscribe from obtaining an internal lock, resulting in a
// deadlock.
func (c *Client) Unsubscribe(msgs chan Message) error {
	c.init()

	c.chanPoolMu.Lock()
	defer c.chanPoolMu.Unlock()

	for i := range c.chanPool {
		if c.chanPool[i] == msgs {
			c.chanPool = append(c.chanPool[:i], c.chanPool[i+1:]...)
			return nil
		}
	}

	return ErrNotSubscribed
}

// GetMessageStream returns a newly-created channel that will receive real-time
// Slack messages, as well as a channel that the caller may close to indicate
// that it wishes to stop processing values. The msgs channel will be closed
// when the associated Client is closed, or some time after done is closed.
//
// The returned channel includes a small buffer, but all sends into it are
// blocking. The caller MUST continuously receive and process values from the
// msgs channel until it is closed, or processing will slow down for all
// consumers. This applies even after the done channel is closed. The intent
// of this behavior is to prevent dropped and out-of-order messages.
//
// Deprecated: Use Subscribe and Unsubscribe instead.
func (c *Client) GetMessageStream() (msgs <-chan Message, done chan<- struct{}) {
	c.init()

	// Small amount of buffering to maybe speed things up a bit?
	msgsRW, doneRW := make(chan Message, 1), make(chan struct{})
	c.Subscribe(msgsRW) // always returns nil

	// When we get a done signal, remove this channel from the pool
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()

		select {
		case <-doneRW:
		case <-c.done:
		}

		c.Unsubscribe(msgsRW) // always returns nil
		close(msgsRW)
	}()

	return msgsRW, doneRW
}

// SendMessage sends the given Message to its associated Slack channel.
func (c *Client) SendMessage(m Message) {
	c.init()

	msg := c.rtm.NewOutgoingMessage(m.Text, m.ChannelID)
	c.rtm.SendMessage(msg)
}

// Close shuts down this Client and closes all channels that have been created
// from it using GetMessageStream, blocking until these closures are finished.
// The behavior of other Client methods after Close has returned is undefined.
func (c *Client) Close() error {
	c.init()

	close(c.done)
	c.wg.Wait()

	// Allow unit testing of the closure logic
	if c.rtm != nil {
		return c.rtm.Disconnect()
	}

	return nil
}
