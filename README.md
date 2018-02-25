# slackio

[![GoDoc](https://godoc.org/github.com/ahamlinman/slackio?status.svg)](https://godoc.org/github.com/ahamlinman/slackio)
[![Build Status](https://travis-ci.org/ahamlinman/slackio.svg?branch=master)](https://travis-ci.org/ahamlinman/slackio)

**slackio** implements real-time Slack communication behind Go's [io.Reader]
and [io.Writer] interfaces.

[io.Reader]: https://golang.org/pkg/io/#Reader
[io.Writer]: https://golang.org/pkg/io/#Writer

## Usage

It is recommended that you manage your project's dependencies with [`dep`].
This repository includes a manifest with tested versions of slackio's
underlying dependencies, which `dep` will respect when including slackio as a
dependency.

[`dep`]: https://github.com/golang/dep

## Development

1. `git clone https://github.com/ahamlinman/slackio.git`
1. `make test`, etc.

## Example

A small program is provided in the `example/` directory to demonstrate the
capabilities of slackio. To use it, set the `SLACK_TOKEN` environment variable
to a valid Slack API token. Then, run in one of two modes:

* With `go run example/main.go`, the contents of all available channels are
  streamed to standard output.
* With `go run example/main.go <channel ID>`, the contents of a single channel
  are streamed to standard output. Lines read from standard input are sent back
  to the channel as messages.

## Status and Stability

As of November 2017, the key desired functionalities of package slackio have
been implemented. Feature development is on an indefinite hiatus, but
maintenance updates (e.g. bug fixes) may be made from time to time.

Until v1.0.0 is tagged (no guarantees about when or if this will happen), this
project adheres to a scheme based on [Semantic Versioning] as follows:

* MINOR updates could potentially (but not necessarily) contain breaking
  changes
* PATCH updates will not contain breaking changes

All notable changes will be documented in CHANGELOG.md.

[Semantic Versioning]: http://semver.org/spec/v2.0.0.html

## License

MIT (see LICENSE.txt)
