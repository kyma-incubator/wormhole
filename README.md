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
$ openssl req --x509 --newkey rsa:4096 --days 365 --nodes --keyout connector-key.pem --out connector.pem
```

## Running wormhole-connector

You'll need to provide the address to the other end of the HTTP/2 tunnel.
You can find an example of the component sitting on the other side in [wormhole-dispatcher](examples/wormhole-dispatcher).

You'll also need to provide a CA file that signed the certificates used by the component on the other side.

```
$ ./wormhole-connector --trust-ca-file ca.pem --kyma-server https://dispatcher.wormhole.example.com
```

Then you can configure your applications to use `https://localhost:8080` as an HTTPS proxy.
