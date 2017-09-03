package slackio

import (
	"fmt"
	"io"
	"os"
)

func ExampleReader() {
	// Stream messages from all of a user's Slack channels to stdout

	client := NewClient("xoxb-slack-api-token")
	reader := NewReader(client, "")

	io.Copy(os.Stdout, reader)
}

func ExampleWriter() {
	// Write a short message to a Slack channel

	client := NewClient("xoxb-slack-api-token")
	writer := &Writer{Client: client, SlackChannelID: "C12345678"}

	_, err := writer.Write([]byte("hi\n"))
	if err != nil {
		fmt.Println(err.Error())
	}
}
