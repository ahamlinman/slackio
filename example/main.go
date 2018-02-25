package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/ahamlinman/slackio"
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
