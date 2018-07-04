# A simple Kyma Server for testing

## Generating code for Kyma server

```
$ wget http://central.maven.org/maven2/org/openapitools/openapi-generator-cli/3.0.2/openapi-generator-cli-3.0.2.jar \
  -O $HOME/bin/openapi-generator-cli.jar
$ cd $GOPATH/src/github.com/kinvolk/wormhole-connector/examples
$ java -jar ~/bin/openapi-generator-cli.jar generate -l go-server \
  -i ./kyma-api/kyma-externalapi.yaml -o ./kyma-server
```

Then you can see the generated code under the directory `kyma-server`.

## Running the server
To run the server, follow these simple steps:

```
$ cd $GOPATH/src/github.com/kinvolk/wormhole-connector/examples/kyma-server
$ go run ./main.go
```

Test its REST API, for example:

```
curl -v --insecure --http2 --tlsv1 --sslv2 https://localhost:8080//v1/health
```

### Running the server in a docker container

To run the server in a docker container:

```
$ cd $GOPATH/src/github.com/kinvolk/wormhole-connector/examples/kyma-server
$ docker build --network=host -t kyma-server .
```

Once image is built use

```
docker run --rm -it kyma-server
```

Then the server will listen on `172.17.0.2:8080` by default.

Test its REST API, for example:

```
curl -v --insecure --http2 --tlsv1 --sslv2 https://172.17.0.2:8080//v1/health
```

