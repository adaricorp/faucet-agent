# faucet-agent

This tool can ingest streaming network events from the
(faucet)[https://github.com/faucetsdn/faucet] event API
and push this data to a prometheus server.

## Requirements

### Faucet configuration

The following environment variable must be set and contain the path where the event socket
should be written, e.g:

```
FAUCET_EVENT_SOCK=/run/faucet/event.sock
```

## Downloading

Download prebuilt binaries from [GitHub](https://github.com/adaricorp/faucet-agent/releases/latest).

## Running

To connect faucet-agent with a faucet event socket with path /run/faucet/event.sock
and push network events to a prometheus endpoint:

```
faucet_agent \
    --event-socket /run/faucet/event.sock \
    --prometheus-remote-write-uri http://127.0.0.1:9090/api/v1/write \
```

It is also possible to configure faucet-agent by using environment variables:

```
FAUCET_AGENT_EVENT_SOCKET="/tmp/faucet.sock" faucet_agent
```

## Metrics

### Prometheus

All metric names for prometheus start with `faucet_`.
