# gliderlabs/ssh

[![Slack](https://slack.gliderlabs.com/badge.svg)](http://slack.gliderlabs.com) [![GoDoc](https://godoc.org/github.com/gliderlabs/ssh?status.svg)](https://godoc.org/github.com/gliderlabs/ssh) [![Go Report Card](https://goreportcard.com/badge/github.com/gliderlabs/ssh)](https://goreportcard.com/report/github.com/gliderlabs/ssh)

This Go package wraps the [crypto/ssh
package](https://godoc.org/golang.org/x/crypto/ssh) with a higher-level API for
building SSH servers. The goal of the API was to make it as simple as using
[net/http](https://golang.org/pkg/net/http/), so the API is very similar:

```
 package main
 
 import (
     "github.com/gliderlabs/ssh"
     "io"
     "log"
 )
 
 func main() {
     ssh.Handle(func(s ssh.Session) {
         io.WriteString(s, "Hello world\n")
     })  
 
     log.Fatal(ssh.ListenAndServe(":2222", nil))
 }

```

This package was built after working on nearly a dozen projects using SSH and
collaborating with [@shazow](https://twitter.com/shazow) (known for [ssh-chat](https://github.com/shazow/ssh-chat)).

## Usage

[See GoDoc reference.](https://godoc.org/github.com/gliderlabs/ssh)

## Testing

We could use some help figuring out the best way to test this library. Since
there is very little functionality it's adding, it doesn't seem appropriate to
duplicate the crypto/ssh tests, however, maybe that's actually the best idea. Perform
the same tests using this API.

## Contributing

Pull requests are welcome! However, since this project is very much about API
design, please submit API changes as issues to discuss before submitting PRs.

Also, you can [join our Slack](http://slack.gliderlabs.com) to discuss as well.

## License

BSD
