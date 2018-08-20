# Serf/Raft example

This example shows how the Wormhole Connector cluster can store event information in a replicated fashion using Raft.

We'll assume certificates are generated in `$PWD/connector.pem` and `$PWD/connector-key.pem` as mentioned in the [README](../README.md#generate-certificates).

## Get the wormhole connector cluster running

### With docker

This requires [docker/moby](https://github.com/moby/moby/releases).

First, build an image:

```
$ make docker
```

This will generate a docker image with a name `kyma-incubator/wormhole-connector`.

Now you can start the first server of your cluster.

```
docker run --rm -i --tty \
    --env=WORMHOLE_BIND_INTERFACE=eth0 \
    --volume=$PWD/connector.pem:/tmp/connector.pem \
    --volume=$PWD/connector-key.pem:/tmp/connector-key.pem \
    --volume=$HOME/.config/wormhole-connector:/tmp/.config/wormhole-connector \
    --entrypoint=/bin/sh \
    kyma-incubator/wormhole-connector \
    -c "/entrypoint.sh"
```

Now we'll start the rest of the servers pointing to the IP of the first member, whose IP address is printed via stdout of the first server launched above. Let's assume that the IP is `172.17.0.2`.

Run this twice to have a 3-node cluster:

```
docker run --rm -i --tty \
    --env=WORMHOLE_BIND_INTERFACE=eth0 \
    --volume=$PWD/connector.pem:/tmp/connector.pem \
    --volume=$PWD/connector-key.pem:/tmp/connector-key.pem \
    --volume=$HOME/.config/wormhole-connector:/tmp/.config/wormhole-connector \
    --entrypoint=/bin/sh \
    kyma-incubator/wormhole-connector \
    -c "/entrypoint.sh --serf-member-addrs=172.17.0.2:1111"
```

We now have a Wormhole Connector cluster running.

## With rkt

This requires [rkt](https://github.com/rkt/rkt/releases) and [build](https://github.com/containers/build/releases).

First, build an ACI image:

```
$ make aci
```

This will generate an ACI file named `wormhole-connector-linux-amd64.aci` in the `./aci` directory of the repository.

Now you can start the first server of your cluster.

```
$ sudo rkt \
    --insecure-options=image \
    run \
    --set-env=HOME=/tmp \
    --set-env=WORMHOLE_BIND_INTERFACE=eth0 \
    --volume cert,kind=host,source=$PWD/connector.pem \
    --volume key,kind=host,source=$PWD/connector-key.pem \
    --volume config,kind=host,source=$HOME/.config/wormhole-connector \
    --mount volume=config,target=$HOME/.config/wormhole-connector \
    aci/wormhole-connector-linux-amd64.aci
```

Now we'll start the rest of the servers pointing to the IP of the first member, let's find out that IP:

```
$ rkt list | grep running
0f3359dc	wormhole-connector	kyma-incubator.io/wormhole-connector	running	5 seconds ago	4 seconds ago	default:ip4=172.16.28.2
```

It's `172.16.28.2`, so we can now start the rest.

Run this twice to have a 3-node cluster:

```
$ sudo rkt \
    --insecure-options=image \
    run \
    --set-env=HOME=/tmp \
    --set-env=WORMHOLE_BIND_INTERFACE=eth0 \
    --volume cert,kind=host,source=$PWD/connector.pem \
    --volume key,kind=host,source=$PWD/connector-key.pem \
    --volume config,kind=host,source=$HOME/.config/wormhole-connector \
    --mount volume=config,target=$HOME/.config/wormhole-connector \
    aci/wormhole-connector-linux-amd64.aci \
    -- \
    --serf-member-addrs 172.16.28.2:1111
```

We now have a Wormhole Connector cluster running.

## Store events

The Wormhole Connector currently implements a simple queue for storing events, let's test it.
For now, an event is just a string, but this can work with any data with some small code modifications.

We'll use curl to demonstrate this, with the `-k` flag to skip certificate checks and make things simple.

We first store an event:

```
$ curl -k -X POST -d event="event1" https://172.17.0.2:8080/event
```

We can now get the queue front:

```
$ curl -k https://172.17.0.2:8080/event
event1
```

We can also see the event in a Follower node:

```
$ curl -k https://172.17.0.3:8080/event
event1
```

Let's add more events, note we can also add them in the Follower node because the Wormhole Connector redirects write requests to the leader (so we need to pass the `-L` flag to curl to make it follow redirects):

```
$ curl -kL -X POST -d event="event2" https://172.17.0.3:8080/event
$ curl -kL -X POST -d event="event3" https://172.17.0.4:8080/event
```

From now on, we'll either send write requests to the Leader or to one of the Followers with curl's `-L` flag.

If we get the queue front, we'll see the same event because we haven't dequeued it:

```
$ curl -k https://172.17.0.3:8080/event
event1
```

Let's dequeue it with a DELETE request:

```
$ curl -k -X DELETE https://172.17.0.2:8080/event
$ curl -k https://172.17.0.3:8080/event
event2
```

We can continue until the queue is empty

```
$ curl -k -X DELETE https://172.17.0.2:8080/event
$ curl -k https://172.17.0.3:8080/event
event3
$ curl -k -X DELETE https://172.17.0.2:8080/event
$ curl -k https://172.17.0.3:8080/event

```

We'll add an event for the next section:

```
curl -kL -X POST -d event="survivorEvent" https://172.17.0.4:8080/event
```

### Killing the leader

When killing the leader, a new leader should be elected and things should work fine when connecting to one of the remaining nodes.

Let's find the leader (oldest container) and stop it:

```
$ docker ps
CONTAINER ID        IMAGE                        COMMAND                  CREATED              STATUS              PORTS                     NAMES
bd842fe90782        kyma-incubator/wormhole-connector   "/bin/sh -c '/entryp…"   3 seconds ago        Up 2 seconds        1111-1112/tcp, 8080/tcp   agitated_dubinsky
796196f6ccb5        kyma-incubator/wormhole-connector   "/bin/sh -c '/entryp…"   About a minute ago   Up About a minute   1111-1112/tcp, 8080/tcp   adoring_poincare
b2655aa28aef        kyma-incubator/wormhole-connector   "/bin/sh -c '/entryp…"   2 minutes ago        Up 2 minutes        1111-1112/tcp, 8080/tcp   vigilant_lamport
$ docker stop b2655aa28aef
b2655aa28aef
```

Or, with rkt:

```
$ rkt list | grep running
0f3359dc	wormhole-connector	kyma-incubator.io/wormhole-connector	running	7 minutes ago	7 minutes ago	default:ip4=172.16.28.2
5293dad1	wormhole-connector	kyma-incubator.io/wormhole-connector	running	5 minutes ago	5 minutes ago	default:ip4=172.16.28.3
d9ffd255	wormhole-connector	kyma-incubator.io/wormhole-connector	running	5 minutes ago	5 minutes ago	default:ip4=172.16.28.4
$ sudo rkt stop 0f3359dc
0f3359dc-2555-465b-9a2a-3dac255fa769
```

The cluster should still be functional and the event we added in the previous section should still be there:

```
$ curl -k https://172.17.0.3:8080/event
survivorEvent
$ curl -kL -X POST -d event="eventLeaderKilled" https://172.17.0.3:8080/event
$ curl -k https://172.17.0.3:8080/event
survivorEvent
$ curl -kL -X DELETE https://172.17.0.4:8080/event
$ curl -k https://172.17.0.4:8080/event
eventLeaderKilled
```

## Tips and tricks

### Cleaning up rkt containers and CNI configurations

Sometimes you would want to reset the test environment to start testing it again from scratch.
Then you might want to clean up CNI config files as well as existing rkt containers.

```
$ sudo rkt gc --grace-period=0
```

After that, a new rkt instance will have a fresh IP address starting from the first IP `172.16.28.2`.
