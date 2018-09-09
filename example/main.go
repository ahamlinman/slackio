/*

Example is a small demonstration of slackio's capabilities.

It has two modes.

Without a channel ID, it will connect to Slack and print (to standard output)
all messages from all channels that the user is currently a member of.

With a channel ID, it will connect to Slack and print (to standard output) all
messages from a single channel. Additionally, lines read from standard input
are sent back to the channel as messages.

Note that the channel ID is a 9-character identifier, and is not the same as
the human-readable channel name in the Slack UI. When viewing Slack in a
browser, the ID of the selected channel will appear at the end of the URL path.

Usage:

	SLACK_TOKEN=xoxb-token go run example/main.go [channel]

The SLACK_TOKEN environment variable must be a valid Slack API token.
Otherwise, example will panic.

*/
package main // import "go.alexhamlin.co/slackio/example"

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"go.alexhamlin.co/slackio"
)

func main() {
	apiToken := os.Getenv("SLACK_TOKEN")
	if apiToken == "" {
		fmt.Fprintln(os.Stderr, "fatal: SLACK_TOKEN environment variable not set")
		os.Exit(1)
	}

	var channelID string
	if len(os.Args) > 1 {
		channelID = os.Args[1]
		fmt.Fprintf(os.Stderr, "(connecting to channel %s for reading and writing)\n", channelID)
	} else {
		fmt.Fprintln(os.Stderr, "(connecting to all channels for reading only)")
	}

	client := slackio.NewClient(apiToken)
	defer client.Close()

	reader := slackio.NewReader(client, channelID)
	defer reader.Close()

	go func() {
		if _, err := io.Copy(os.Stdout, reader); err != nil {
			panic(err)
		}
	}()

	if channelID == "" {
		io.Copy(ioutil.Discard, os.Stdin)
		return
	}

	writer := slackio.NewWriter(client, channelID, nil)
	defer writer.Close()

	if _, err := io.Copy(writer, os.Stdin); err != nil {
		panic(err)
	}
}
