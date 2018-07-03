# Wormhole - simple HTTP/2 connector for Kyma clusters

## Building wormhole-connector

To build `wormhole-connector`, simply run:

```
$ make
```

To clean up binaries:

```
$ make clean
```

## Generate certificates

Before running wormhole-connector, you need to generate a self-signed certificate and its key.

```
$ openssl req --x509 --newkey rsa:4096 --days 365 --nodes --keyout server.key --out server.crt
```

## Running wormhole-connector

Simply run a binary `wormhole-connector`.

```
$ ./wormhole-connector
```

Then its REST API will be available via https://localhost:8080/.
