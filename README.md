# Observability library for Go

This library allows you to send semi-structured observations to Observe from
your go program. It will asynchronously buffer and post them in the background,
to avoid blocking your main program.

## Installation

Install the library:

```
go get github.com/observeinc/o11y
```

## Initialization

Start the library in your `main()`:

```
import (
    "fmt"
    "os"

    "github.com/observeinc/o11y"
)

func main() {
 ...
    // The 'identifier' defaults to argv[0], but you can set it to something
    // more interesting. There's a few other options you might want to look at
    // in o11y/client.go, too.
    o11y, err := o11y.NewClient(ctx, o11y.WithIdentifier(os.Getenv("HOSTNAME")))
    if err != nil {
        fmt.Fprintf(os.Stderr, "%w\n", err)
        os.Exit(1)
    }
 ...
}
```

## Use

Send things that are JSON encodable to Observe:

```
type MyEvent struct {
    Name  string         `json:"name"`
    Value float64        `json:"value"`
    Props map[string]any `json:"props"`
}

func DoTheThing(cli o11y.Client) {
 ...
    // This will return an error if the client backs up and starts
    // shedding observations. The number is the verbosity level --
    // higher numbers can be filtered with the WithVerbosity() option.
    // The default verbosity is 5, so numbers 6 and up will be
    // discarded. Any JSON encodable object can be sent. If you pass
    // a non-JSON-encodable object, an error will be printed to
    // os.Stderr.
    cli.SendJSON(3, MyEvent{
        Name: "Bob",
        Value: 42.0,
        Props: map[string]any{"rad":true},
    })
 ...
}
```

## Notes

You will need to set your keys in the environment (or provide them as options
to the client when creating it):

```
$ export OBSERVE_URL='https://123456789012.collect.observeinc.com/v1/http/my-thing'
$ export OBSERVE_AUTHTOKEN='ds1sdlkfjslslkfj:ldfkjslkjfslkjfdslkjfdslkfjsd'
$ go run my-thing
```

To generate authtokens for a datastream, open the datastream in the sidebar,
and press "New Token". Note that you can only copy it once when first creating
it, so make sure to save it when you do! (Else you have to create another one,
and then where would you be?!)

## Connectivity

The library will by default verify that it can post to the collector service
when it starts up, and returns an error if it can't. If you want to disable
this behavior, pass the `WithoutCheckConnect()` option to the `NewClient()`
call.

If you want to make sure you wait for all observations to be posted before
exiting your program, you can call `Close()` on the `o11y.Client`. This will
block until all queued observations have been posted, or the sending re-tries
have exhausted with error.

If you queue more observations that can be sustainably uploaded from your
connection, the library will start shedding load by discarding observations as
they are queued.

## See Also

For more, see [Observe Inc Documentation](https://docs.observeinc.com/)

