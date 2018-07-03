# Wormhole connector - simple HTTP/2 connector for Kyma clusters

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

## how to test the REST API

Open another terminal, test each method for the REST API.

```
# register service
$ curl -v -X POST --insecure --http2 https://localhost:8080/v1/metadata/services

# get services
$ curl -v -X GET --insecure --http2 https://localhost:8080/v1/metadata/services

# get service by ID
$ curl -v -X GET --insecure --http2 https://localhost:8080/v1/metadata/services/id1

# delete service
$ curl -v -X DELETE --insecure --http2 https://localhost:8080/v1/metadata/services/id1

# update service
$ curl -v -X PUT --insecure --http2 https://localhost:8080/v1/metadata/services/id1

# get health
$ curl -v -X GET --insecure --http2 https://localhost:8080/v1/health

# publish events
$ curl -v -X POST --insecure --http2 https://localhost:8080/v1/events
```
