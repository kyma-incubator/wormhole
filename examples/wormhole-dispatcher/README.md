# wormhole-dispatcher

wormhole-dispatcher is a component designed to be on the server side of an HTTP/2 tunnel from wormhole-connector.
It will route the connections received through the tunnel to hosts within its reach.

Usually, wormhole-dispatcher will be running inside a Kubernetes cluster and will be expose to the outside.
In this way, outside applications can reach services inside the Kubernetes cluster by using wormhole-connector as an HTTP proxy server.

## Building

You can build a static binary and containerize it yourself with:

```
make linux
```

The binary expects a certificate and key files named `dispatcher.pem` and `dispatcher-key.pem` in the same directory.

Or you can build a docker image directly with the example certificate and key files:

```
make
```

After that you can tag it and push it to your Docker registry so you can use it in your Kubernetes cluster.

## Deploying

In the `kubernetes/` directory you can find example manifests to deploy the wormhole-dispatcher and expose it as a Kubernetes service.
