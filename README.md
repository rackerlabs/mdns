# mdns

[![Build Status](https://travis-ci.org/rackerlabs/mdns.svg?branch=master)](https://travis-ci.org/rackerlabs/mdns)

Welcome to `designate-mdns`, rewritten in Go.

```shell
$ mdns --help
  -allowUnknownFlags
        Don't terminate the app if ini file contains unknown flags.
  -bind_address string
        IP to listen on (default "127.0.0.1")
  -bind_port string
        port to listen on (default "5358")
  -config string
        Path to ini config for using in go flags. May be relative to the current executable path.
  -configUpdateInterval duration
        Update interval for re-reading config file set via -config flag. Zero disables config file re-reading.
  -db string
        db connection string (default "root:password@tcp(127.0.0.1:3306)/designate")
  -debug
        enables debug mode
  -dumpflags
        Dumps values for all flags defined in the app into stdout in ini-compatible syntax and terminates the app.
  -version
        prints version information
```

There is a key assumption that this code makes that makes it different from
the Python code. This _does not_ listen on RabbitMQ for RPC calls from
Designate. This is simply a translation between DNS Protocol and the
Designate database.

This is an intentional decision. Because while it would certainly be possible
to figure out how to answer to `oslo_messaging`, it's undesirable to chase
RPC endpoints/versions. The [worker model](https://review.openstack.org/#/c/258621/)
spec details a world without the need for a MiniDNS that sends NOTIFYs and
queries nameservers. This is that.

## Setup

It's pretty easy to get up and running, set up your Go working tree and clone
this into `src/github.com/rackerlabs/mdns`.

Dependencies are managed with [Glide](https://github.com/Masterminds/glide)
so you'll need to install it `brew install glide`. Then `glide install`.

It accepts a config file with the `-config` flag. `-help` will show you
what you need to configure + the defaults.

## Is it Fast?

Yes. Listening on localhost, with the same Designate database, here are some
basic benchmarks against the current `designate-mdns`. These were created using
the `bench` tool located in `/cmd`.

|                | SOA Queries      | 2K Record AXFR |
|----------------|------------------|----------------|
| designate-mdns | 40 qps, ~1.2s    | 2.5 qps, ~2.3s |
| mdns           | 2000 qps, ~.016s | 20 qps, ~.4s   |
