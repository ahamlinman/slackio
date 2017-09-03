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
	rtm        *slack.RTM
	wg         sync.WaitGroup
	done       chan struct{}
	chanPool   []chan Message
	chanPoolMu sync.Mutex
}

// NewClient returns a new Client that connects to Slack using the given API
// token.
func NewClient(apiToken string) *Client {
	if apiToken == "" {
		panic("slackio: Client requires a non-blank API token")
	}

	c := &Client{}
	c.done = make(chan struct{})

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

// distribute fans out a message to all subscribed channels.
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

// SendMessage sends the given Message to its associated Slack channel.
func (c *Client) SendMessage(m Message) {

	msg := c.rtm.NewOutgoingMessage(m.Text, m.ChannelID)
	c.rtm.SendMessage(msg)
}

// Close shuts down this Client and disconnects it from Slack, effectively
// unsubscribing all channels from the message stream.
func (c *Client) Close() error {
	close(c.done)
	c.wg.Wait()
	return c.rtm.Disconnect()
}
