package slackio

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
)

type errorReader struct{}

var errorReaderErr = errors.New("sample reader error")

func (errorReader) Read(_ []byte) (int, error) {
	return 0, errorReaderErr
}

type testBatch struct {
	out string
	err error
}

type testBatcher struct {
	batches  []testBatch
	next     chan struct{}
	received chan struct{}
}

func (b *testBatcher) makeBatcher() Batcher {
	b.next, b.received = make(chan struct{}), make(chan struct{})

	return func(_ io.Reader) (<-chan string, <-chan error) {
		outCh, errCh := make(chan string), make(chan error, 1)

		go func() {
			for _, batch := range b.batches {
				<-b.next

				if batch.out != "" {
					outCh <- batch.out
				}

				if batch.err != nil {
					errCh <- batch.err
				}

				b.received <- struct{}{}
			}

			<-b.next
			close(outCh)
			close(errCh)
			b.received <- struct{}{}
		}()

		return outCh, errCh
	}
}

func (b *testBatcher) emitNext() {
	b.next <- struct{}{}
	<-b.received
}

func TestLineBatcher(t *testing.T) {
	cases := []struct {
		description string
		input       io.Reader
		output      []string
		err         error
	}{
		{
			"reads single lines without termination",
			strings.NewReader("test string"),
			[]string{"test string"},
			nil,
		},
		{
			"reads multiple lines",
			strings.NewReader("test\nstrings\nmany\nlines\n"),
			[]string{"test", "strings", "many", "lines"},
			nil,
		},
		{
			"passes errors through",
			errorReader{},
			nil,
			errorReaderErr,
		},
	}

	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			var actualOutput []string
			outCh, errCh := LineBatcher(tc.input)

			for s := range outCh {
				actualOutput = append(actualOutput, s)
			}

			if !reflect.DeepEqual(actualOutput, tc.output) {
				t.Errorf("unexpected LineBatcher output %#v (expected %#v)", actualOutput, tc.output)
			}

			if e := <-errCh; e != tc.err {
				t.Errorf("unexpected LineBatcher error %#v (expected %#v)", e, tc.err)
			}
		})
	}
}

func TestIntervalBatcher(t *testing.T) {
	tb := &testBatcher{
		batches: []testBatch{
			{out: "test"},
			{out: "messages"},
			{out: "to"},
			{out: "batch"},
		},
	}

	timeCh := make(chan time.Time)
	timeAfter = func(_ time.Duration) <-chan time.Time { return timeCh }
	defer func() { timeAfter = time.After }()

	batcher := NewIntervalBatcher(tb.makeBatcher(), time.Second, " ")
	outCh, errCh := batcher(strings.NewReader(""))

	tb.emitNext()
	tb.emitNext()
	timeCh <- time.Now()
	if s := <-outCh; s != "test messages" {
		t.Fatalf("unexpected interval batcher output: %q (expected 'test messages')", s)
	}

	tb.emitNext()
	tb.emitNext()
	timeCh <- time.Now()
	if s := <-outCh; s != "to batch" {
		t.Fatalf("unexpected interval batcher output: %q (expected 'to batch')", s)
	}

	tb.emitNext() // close output channel to stop downstream batcher
	if _, ok := <-outCh; ok {
		t.Fatal("interval batcher did not close output when upstream did")
	}

	if err := <-errCh; err != nil {
		t.Fatalf("unexpected interval batcher error: %q", err.Error())
	}
}

func TestIntervalBatcherHandlesErrors(t *testing.T) {
	expectedErr := errors.New("test batcher error")
	tb := &testBatcher{
		batches: []testBatch{
			{out: "test"},
			{out: "messages"},
			{err: expectedErr},
		},
	}

	timeCh := make(chan time.Time)
	timeAfter = func(_ time.Duration) <-chan time.Time { return timeCh }
	defer func() { timeAfter = time.After }()

	batcher := NewIntervalBatcher(tb.makeBatcher(), time.Second, " ")
	outCh, errCh := batcher(strings.NewReader(""))

	for range tb.batches {
		tb.emitNext()
	}
	tb.emitNext() // close output channel to stop downstream batcher

	if s := <-outCh; s != "test messages" {
		t.Fatalf("interval batcher did not flush all output on error: %q (expected 'test messages')", s)
	}

	if _, ok := <-outCh; ok {
		t.Fatal("interval batcher did not close output when upstream did")
	}

	if err := <-errCh; err != expectedErr {
		t.Fatalf("unexpected interval batcher error: %q", err.Error())
	}
}
