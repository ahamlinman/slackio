package slackio

import "sync"

// subscription is an internal type that is tightly bound to Client and helps
// simplify management tasks.
type subscription struct {
	client *Client
	id     int
	ch     chan Message
	done   chan struct{}
	wg     sync.WaitGroup
}

func newSubscription(client *Client, id int, ch chan Message) *subscription {
	s := &subscription{
		client: client,
		id:     id,
		ch:     ch,
		done:   make(chan struct{}),
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.process()
	}()

	return s
}

func (s *subscription) active() bool {
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

func (s *subscription) process() {
	// Other than the read lock at the top of the loop, all potentially blocking
	// operations should terminate early when s.done is closed. Take care to
	// ensure this happens.

	// This function is designed to run in a goroutine (see newSubscription).

	for s.active() {
		s.client.messagesLock.RLock()

		if len(s.client.messages) > 0 {
			// Check if we are trying to get a past message. If so, this consumer has
			// fallen way behind, and we will skip them to the end of the queue. This
			// could arguably be handled better, but it should be a rare case.
			if s.id < s.client.messages[0].ID {
				s.id = s.client.messages[len(s.client.messages)-1].ID + 1
				s.client.messagesLock.RUnlock()
				continue
			}

			// Next, check if the message we are trying to get is in the queue right
			// now. If so, pick it out and send it to the consumer (making sure not
			// to leave the queue locked, in case the send blocks). Then prepare to
			// move on to the next message in line.
			if s.id <= s.client.messages[len(s.client.messages)-1].ID {
				idx := s.id - s.client.messages[0].ID
				msg := s.client.messages[idx]
				s.client.messagesLock.RUnlock()

				select {
				case s.ch <- msg:
				case <-s.done:
				}

				s.id++
				continue
			}
		}

		// At this point, we are trying to get a message that does not exist yet.
		// We will wait for it to arrive, but will wrap this with a channel so we
		// can use "select" to terminate early. If the subscription does stop
		// before we finish waiting, this goroutine will terminate on the next
		// message or when Client sends a final broadcast on its own closure (see
		// client.go).
		msgWait := make(chan struct{})
		go func() {
			s.client.messagesCond.Wait()
			s.client.messagesLock.RUnlock()
			close(msgWait)
		}()

		select {
		case <-msgWait:
		case <-s.done:
		}

		// It is possible that our message has arrived at this point. But if not,
		// we will simply come back and wait again as long as the subscription is
		// active.
	}
}

func (s *subscription) stop() {
	close(s.done)
	s.wg.Wait()
}
