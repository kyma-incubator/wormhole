# Kubernetes example

This example shows how to test wormhole-connector to proxy client request to a
Kubernetes cluster, so clients can access services provided by the Kubernetes
cluster transparently.

This diagram shows the components involved:

```
                                                             +----------------------------+
                                                             |     Kubernetes cluster     |
                                                             |                            |
                                                             |                            |
                                                             |                            |
              proxy                                 HTTP/2   |                            |
+--------+  connection    +--------------------+    tunnel   |  +---------------------+   |
| client +<-------------->+ wormhole-connector +<-------------->+ wormhole-dispatcher |   |
+--------+                +--------------------+             |  +----------+----------+   |
                                                             |             ^              |
                                                             |             |     tcp      |
                                                             |             |  connection  |
                                                             |             |              |
                                                             |             v              |
                                                             |      +------+------+       |
                                                             |      | k8s service |       |
                                                             |      +-------------+       |
                                                             |                            |
                                                             +----------------------------+
```

## Generate TLS certificates

We'll use [cfssl](https://github.com/cloudflare/cfssl) to generate the TLS certificates.

### Generate CA

In this section we'll provision a Certificate Authority that can be used to generate additional TLS certificates.

Create and enter a directory named `certs` in the root directory of the repository.

Generate the CA configuration file, certificate, and private key by pasting this in the terminal:

```
{

cat > ca-config.json <<EOF
{
  "signing": {
    "default": {
      "expiry": "8760h"
    },
    "profiles": {
      "wormhole": {
        "usages": ["signing", "key encipherment", "server auth", "client auth"],
        "expiry": "8760h"
      }
    }
  }
}
EOF

cat > ca-csr.json <<EOF
{
  "CN": "Kyma Wormhole",
  "key": {
    "algo": "rsa",
    "size": 2048
  },
  "names": [
    {
      "C": "DE",
      "L": "Berlin",
      "O": "Kyma Wormhole",
      "OU": "DE",
      "ST": "Berlin"
    }
  ]
}
EOF

cfssl gencert -initca ca-csr.json | cfssljson -bare ca

}
```

Results:

```
ca-key.pem
ca.pem
```

### Generate wormhole-connector certificates

Generate the `connector` client certificate and private key by pasting this in the terminal:

```
{

cat > connector-csr.json <<EOF
{
  "CN": "connector.wormhole.io",
  "key": {
    "algo": "rsa",
    "size": 2048
  },
  "names": [
    {
      "C": "DE",
      "L": "Berlin",
      "O": "Kyma",
      "OU": "Wormhole Connector",
      "ST": "Berlin"
    }
  ]
}
EOF

cfssl gencert \
  -ca=ca.pem \
  -ca-key=ca-key.pem \
  -config=ca-config.json \
  -profile=wormhole \
  connector-csr.json | cfssljson -bare connector

}
```

Results:

```
connector-key.pem
connector.pem
```

### Generate wormhole-dispatcher certificates

Generate the `dispatcher` client certificate and private key by pasting this in the terminal:

```
{

cat > dispatcher-csr.json <<EOF
{
  "CN": "dispatcher.wormhole.io",
  "key": {
    "algo": "rsa",
    "size": 2048
  },
  "names": [
    {
      "C": "DE",
      "L": "Berlin",
      "O": "Kyma",
      "OU": "Wormhole Connector",
      "ST": "Berlin"
    }
  ]
}
EOF

cfssl gencert \
  -ca=ca.pem \
  -ca-key=ca-key.pem \
  -config=ca-config.json \
  -profile=wormhole \
  dispatcher-csr.json | cfssljson -bare dispatcher

}
```

Results:

```
dispatcher-key.pem
dispatcher.pem
```

## Build wormhole-connector

Let's build wormhole-connector. In the root directory of the project:

```
make
```

## Start minikube

We'll use [minikube](https://github.com/kubernetes/minikube) to start a Kubernetes cluster:

```
minikube start --memory=3072 --kubernetes-version v1.11.0
```

Once it's finished, you should be able to run `kubectl get nodes`:

```
$ kubectl get nodes
NAME       STATUS    ROLES     AGE       VERSION
minikube   Ready     master    14s       v1.11.0
```

## Deploy wormhole-dispatcher

First, we'll set up the docker client so it connects to the docker daemon inside the minikube VM:

```
eval $(minikube docker-env)
```

Then, in the directory `examples/wormhole-dispatcher`, first we need to copy the certificates generated for the wormhole-dispatcher here:

```
cp ../../certs/dispatcher*pem .
```

Then we can build the docker image:

```
make
```

Finally, we'll deploy wormhole-dispatcher to the Kubernetes cluster

```
kubectl apply -f kubernetes/wormhole-dispatcher.yaml
```

## Start wormhole-connector

We need to find out how to connect to the wormhole-dispatcher and add some entries to `/etc/hosts` so our certificates work.

First, we'll add this line to `/etc/hosts`:

```
127.0.0.1 connector.wormhole.io
```

This will allow us to connect to the wormhole-connector by using the `connector.wormhole.io` URL.

Then, we'll figure out the IP of the minikube VM:

```
$ minikube ip
192.168.99.100
```

We'll now add the corresponding line to `/etc/hosts`:

```
192.168.99.100 dispatcher.wormhole.io
```

Then, we'll find out the ports where the wormhole-dispatcher is exposed:

```
$ kubectl get svc wormhole-dispatcher
NAME                  TYPE       CLUSTER-IP     EXTERNAL-IP   PORT(S)                         AGE
wormhole-dispatcher   NodePort   10.110.72.77   <none>        9090:31329/TCP,9091:32681/TCP   2h
```

So we can access the service at `dispatcher.wormhole.io:31329`.
The other port is to establish reverse-tunnels so the Dispatcher can also act as a proxy for applications inside the Kyma cluster.

We have everything we need to start the wormhole-connector.

We'll go back to the root directory of the project and run it:

```
./wormhole-connector \
    --local-addr localhost:8080 \
    --kyma-server https://dispatcher.wormhole.io:31329 \
    --kyma-reverse-tunnel-port 32681 \
    --cert-file certs/connector.pem \
    --key-file certs/connector-key.pem \
    --trust-ca-file certs/ca.pem
```

The HTTP proxy will run at `https://localhost:8080`, the other end of the HTTP/2 tunnel will be the wormhole-dispatcher, and we tell wormhole-connector to trust our CA.

## Create a sample Kubernetes service

To test we can access services inside the Kubernetes cluster, let's set up a simple echo server, exposing it as an internal Service in Kubernetes.

Create a file named `echoserver.yaml` with this content:

```
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  labels:
    app: echoserver
  name: echoserver
spec:
  replicas: 1
  selector:
    matchLabels:
      app: echoserver
  template:
    metadata:
      labels:
        app: echoserver
    spec:
      containers:
      - image: gcr.io/google_containers/echoserver:1.4
        imagePullPolicy: IfNotPresent
        name: echoserver
---
apiVersion: v1
kind: Service
metadata:
  labels:
    app: echoserver
  name: echoserver
spec:
  type: ClusterIP
  ports:
  - port: 8080
    protocol: TCP
  selector:
    app: echoserver
```

Then deploy it:

```
kubectl apply -f echoserver.yaml
```

## Test proxy with cURL

We should now be able to access the service through the wormhole-connector proxy:

```
$ curl --proxy https://connector.wormhole.io:8080 --proxy-cacert certs/ca.pem http://echoserver:8080
CLIENT VALUES:
client_address=172.17.0.13
command=GET
real path=/
query=nil
request_version=1.1
request_uri=http://echoserver:8080/

SERVER VALUES:
server_version=nginx: 1.10.0 - lua: 10001

HEADERS RECEIVED:
accept=*/*
host=echoserver:8080
user-agent=curl/7.61.0
BODY:
-no body in request-$
```

We can also access any other server reachable from the Kubernetes cluster:

```
$ curl --proxy https://connector.wormhole.io:8080 --proxy-cacert certs/ca.pem https://google.com
<HTML><HEAD><meta http-equiv="content-type" content="text/html;charset=utf-8">
<TITLE>301 Moved</TITLE></HEAD><BODY>
<H1>301 Moved</H1>
The document has moved
<A HREF="https://www.google.com/">here</A>.
</BODY></HTML>
```

## Test Dispatcher proxy from the Kyma cluster

We can start a pod in the Kyma cluster and access services running where the Wormhole Connector is running.

To demonstrate this we'll simply run netcat on the host listening on localhost and we'll access it from an alpine pod running in our minikube cluster.

In one terminal, we start a netcat server:

```
$ nc -l 127.0.0.1 -p 9292
```

In another terminal, we run an alpine pod with the CA passed as an env variable, install curl, add an entry to `/etc/hosts`, and access the netcat server through the Wormhole Dispatcher:

```
$ kubectl run -it alpine --image alpine sh --env="WORMHOLE_CA=$(cat certs/ca.pem)"
/ # echo "$WORMHOLE_CA" > ca.pem
/ # apk update
fetch http://dl-cdn.alpinelinux.org/alpine/v3.8/main/x86_64/APKINDEX.tar.gz
fetch http://dl-cdn.alpinelinux.org/alpine/v3.8/community/x86_64/APKINDEX.tar.gz
v3.8.0-59-g7fd9036fc1 [http://dl-cdn.alpinelinux.org/alpine/v3.8/main]
v3.8.0-56-g8ad5ad9f75 [http://dl-cdn.alpinelinux.org/alpine/v3.8/community]
OK: 9539 distinct packages available
/ # apk add curl
(1/5) Installing ca-certificates (20171114-r3)
(2/5) Installing nghttp2-libs (1.32.0-r0)
(3/5) Installing libssh2 (1.8.0-r3)
(4/5) Installing libcurl (7.61.0-r0)
(5/5) Installing curl (7.61.0-r0)
Executing busybox-1.28.4-r0.trigger
Executing ca-certificates-20171114-r3.trigger
OK: 6 MiB in 18 packages
/ # nslookup wormhole-dispatcher
nslookup: can't resolve '(null)': Name does not resolve

Name:      wormhole-dispatcher
Address 1: 10.108.204.112 dispatcher.wormhole.io
/ # echo "10.108.204.112 dispatcher.wormhole.io" >> /etc/hosts
/ # curl --proxy https://dispatcher.wormhole.io:9090 --proxy-cacert ca.pem http://127.0.0.1:9292
```

We should now see the request in the netcat terminal:

```
GET / HTTP/1.1
Host: 127.0.0.1:9292
User-Agent: curl/7.61.0
Accept: */*

```

We can check that there're only two TCP connections established with netstat:

```
$ sudo netstat -punta | grep $(minikube ip) | grep wormhole
tcp        0      0 192.168.99.1:34328      192.168.99.100:31329    ESTABLISHED 25021/./wormhole-co 
tcp        0      0 192.168.99.1:39766      192.168.99.100:32681    ESTABLISHED 25021/./wormhole-co 
```
