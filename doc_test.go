package slackio

import (
	"fmt"
	"io"
	"os"
)

func ExampleReader() {
	// Stream messages from all of a user's Slack channels to stdout

	client := &Client{APIToken: "xoxb-slack-api-token"}
	reader := &Reader{Client: client}

	io.Copy(os.Stdout, reader)
}

func ExampleWriter() {
	// Write a short message to a Slack channel

	client := &Client{APIToken: "xoxb-slack-api-token"}
	writer := &Writer{Client: client, SlackChannelID: "C12345678"}

	_, err := writer.Write([]byte("hi\n"))
	if err != nil {
		fmt.Println(err.Error())
	}
}
