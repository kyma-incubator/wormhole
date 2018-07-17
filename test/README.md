# Test a wormhole connector cluster using rkt

This example requires [rkt](https://github.com/rkt/rkt/releases) and [build](https://github.com/containers/build/releases).

## Get the wormhole connector cluster running

First, build an ACI:

```
$ make aci
```

This will generate an ACI file named `wormhole-connector-linux-amd64.aci` in the `./aci` directory of the repository.

Now you can start the first server of your cluster.

We'll start it with `GODEBUG=http2debug=1` to check that there's one HTTP/2 connection using several streams.

We also pass `--kyma-server 0.0.0.0:8080` so the wormhole connector listens to external connections:

```
$ sudo rkt \
    --insecure-options=image \
    run \
    --set-env=HOME=/tmp \
    --set-env=WORMHOLE_BIND_INTERFACE=eth0 \
    --set-env=GODEBUG=http2debug=1 \
    --volume key,kind=host,source=$PWD/server.key \
    --volume cert,kind=host,source=$PWD/server.crt \
    --volume config,kind=host,source=$HOME/.config/wormhole-connector \
    --mount volume=config,target=$HOME/.config/wormhole-connector \
    aci/wormhole-connector-linux-amd64.aci \
    -- \
    --kyma-server 0.0.0.0:8080
```

Now we'll start the rest of the servers pointing to the IP of the first member, let's find out that IP:

```
$ rkt list | grep running
0f3359dc	wormhole-connector	kinvolk.io/wormhole-connector	running	5 seconds ago	4 seconds ago	default:ip4=172.16.28.2
```

It's `172.16.28.2`, so we can now start the rest:

```
$ sudo rkt \
    --insecure-options=image \
    run \
    --set-env=HOME=/tmp \
    --set-env=WORMHOLE_BIND_INTERFACE=eth0 \
    --set-env=GODEBUG=http2debug=1 \
    --volume key,kind=host,source=$PWD/server.key \
    --volume cert,kind=host,source=$PWD/server.crt \
    --volume config,kind=host,source=$HOME/.config/wormhole-connector \
    --mount volume=config,target=$HOME/.config/wormhole-connector \
    aci/wormhole-connector-linux-amd64.aci \
    -- \
    --kyma-server 0.0.0.0:8080 \
    --serf-member-addrs 172.16.28.2:1111
```

Now that we have the wormhole connector cluster running we can test it, for example, with the [test http2 client](https://github.com/kinvolk/test-http2/tree/master/client).

## Simple test

Let's connect to the leader:

```
$ GODEBUG=http2debug=1 ./client -goroutines 200  -server_addr https://172.16.28.2:8080
```

This should work and print lots of logs in the client and in the leader container, they should show one connection and multiple streams, for example:

```
...
[19465.852199] wormhole-connector[6]: 2018/07/09 13:56:56 http2: server read frame HEADERS flags=END_STREAM|END_HEADERS stream=179 len=7
[19465.853125] wormhole-connector[6]: 2018/07/09 13:56:56 http2: server encoding header ":status" = "200"
[19465.853698] wormhole-connector[6]: 2018/07/09 13:56:56 http2: server encoding header "content-length" = "0"
[19465.854297] wormhole-connector[6]: 2018/07/09 13:56:56 http2: server encoding header "date" = "Mon, 09 Jul 2018 13:56:56 GMT"
[19465.871165] wormhole-connector[6]: 2018/07/09 13:56:56 http2: server read frame HEADERS flags=END_STREAM|END_HEADERS stream=181 len=7
[19465.872025] wormhole-connector[6]: 2018/07/09 13:56:56 http2: server encoding header ":status" = "200"
[19465.872632] wormhole-connector[6]: 2018/07/09 13:56:56 http2: server encoding header "content-length" = "0"
[19465.873110] wormhole-connector[6]: 2018/07/09 13:56:56 http2: server encoding header "date" = "Mon, 09 Jul 2018 13:56:56 GMT"
```

## Redirection

Let's connect to a follower, the connection will be redirected to the leader.

First we have to find out a follower IP

```
$ rkt list | grep running
0f3359dc	wormhole-connector	kinvolk.io/wormhole-connector	running	7 minutes ago	7 minutes ago	default:ip4=172.16.28.2
5293dad1	wormhole-connector	kinvolk.io/wormhole-connector	running	5 minutes ago	5 minutes ago	default:ip4=172.16.28.3
d9ffd255	wormhole-connector	kinvolk.io/wormhole-connector	running	5 minutes ago	5 minutes ago	default:ip4=172.16.28.4
```

Let's use `172.16.28.4`:

```
$ GODEBUG=http2debug=1 ./client -goroutines 200  -server_addr https://172.16.28.4:8080
```

You should see the connection logs both in the follower container and in the leader container.
As before, there should be one connection and multiple streams.

## Killing the leader

When killing the leader, a new leader should be elected and things should work fine when connecting to one of the remaining followers:

```
$ rkt list | grep running | head -n1
0f3359dc	wormhole-connector	kinvolk.io/wormhole-connector	running	13 minutes ago	13 minutes ago	default:ip4=172.16.28.2
$ sudo rkt stop 0f3359dc
$ GODEBUG=http2debug=1 ./client -goroutines 200  -server_addr https://172.16.28.4:8080
```

## Cleaning up rkt containers and CNI configurations

Sometimes you would want to reset the test environment to start testing it again from scratch.
Then you might want to clean up CNI config files as well as existing rkt containers.

```
$ sudo rkt gc --grace-period=0
```

After that, a new rkt instance will have a fresh IP address starting from the first IP `172.16.28.2`.

# Test a wormhole connector cluster using docker/moby

This example requires [docker/moby](https://github.com/moby/moby/releases).

## Get the wormhole connector cluster running

First, build an image:

```
$ make docker
```

This will generate a docker image with a name `kinvolk/wormhole-connector`.

Now you can start the first server of your cluster.

We'll start it with `GODEBUG=http2debug=1` to check that there's one HTTP/2 connection using several streams.

We also pass `--kyma-server=0.0.0.0:8080` so the wormhole connector listens to external connections:

```
docker run --rm --tty \
    --env=WORMHOLE_BIND_INTERFACE=eth0 \
    --env=GODEBUG=http2debug=1 \
    --volume=$PWD/server.crt:/tmp/server.crt \
    --volume=$PWD/server.key:/tmp/server.key \
    --volume=$HOME/.config/wormhole-connector:/tmp/.config/wormhole-connector \
    --entrypoint=/bin/sh \
    kinvolk/wormhole-connector \
    -c "/entrypoint.sh --kyma-server=0.0.0.0:8080"
```

Now we'll start the rest of the servers pointing to the IP of the first member, whose IP address is printed via stdout of the first server launched above. Let's assume that the IP is `172.17.0.2`.

```
docker run --rm --tty \
    --env=WORMHOLE_BIND_INTERFACE=eth0 \
    --env=GODEBUG=http2debug=1 \
    --volume=$PWD/server.crt:/tmp/server.crt \
    --volume=$PWD/server.key:/tmp/server.key \
    --volume=$HOME/.config/wormhole-connector:/tmp/.config/wormhole-connector \
    --entrypoint=/bin/sh \
    kinvolk/wormhole-connector \
    -c "/entrypoint.sh --kyma-server=0.0.0.0:8080 --serf-member-addrs=172.17.0.2:1111"
```

Now that we have the wormhole connector cluster running we can test it, for example, with the [test http2 client](https://github.com/kinvolk/test-http2/tree/master/client).
You can follow the rest of the [testing steps described above](https://github.com/kinvolk/wormhole-connector/tree/master/test#simple-test).
