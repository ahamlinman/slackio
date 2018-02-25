# Changelog

All notable changes to this project will be documented in this file. The format
is based on [Keep a Changelog].

Until v1.0.0 is tagged (no guarantees about when or if this will happen), this
project adheres to a scheme based on [Semantic Versioning] as follows:

* MINOR updates could potentially (but not necessarily) contain breaking
  changes
* PATCH updates will not contain breaking changes

[Keep a Changelog]: http://keepachangelog.com/en/1.0.0/
[Semantic Versioning]: http://semver.org/spec/v2.0.0.html

## [Unreleased]
### Added
- Example program to demonstrate slackio usage (see README.md for details)

### Changed
- When Writer receives an error from its Batcher, it now returns the error on
  Close rather than panicking with it
- When Client receives an authentication error from Slack, it now panics with
  an error value rather than a string (note that this panic occurs in a
  goroutine and is unrecoverable)

## [v0.1.1] - 2018-02-24
### Added
- `Gopkg.toml` manifest for use with the `dep` tool

## [v0.1.0] - 2018-02-22
### Changed
- Initial split-out and versioned release of slackio as a separate package from
  the slackbridge CLI

[Unreleased]: https://github.com/ahamlinman/slackio/compare/v0.1.1...HEAD
[v0.1.1]: https://github.com/ahamlinman/slackio/tree/v0.1.1
[v0.1.0]: https://github.com/ahamlinman/slackio/tree/v0.1.0
