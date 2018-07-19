# Test a standalone wormhole connector with K8s Statefulsets

First of all, make sure that you have [minikube](https://github.com/kubernetes/minikube/releases) installed.

## Push a docker image to a local registry

You need to enable an add-on `registry`, as it's disabled by default.
Start a minikube cluster, import its docker-related environmental variables to your shell.
Then you will be able to push any image to the local registry inside the minikube VM.

```
$ minikube addons enable registry
$ minikube start
$ eval $(minikube docker-env)
```

Build a docker image from the wormhole-connector repository, using `Dockerfile.standalone`, and push it to the private registry.

```
$ make linux
$ REGADDR=$(kubectl -n kube-system get svc registry -o jsonpath="{.spec.clusterIP}")
$ docker build -f Dockerfile.standalone -t $REGADDR/kinvolk/wormhole-connector .
$ docker push $REGADDR/kinvolk/wormhole-connector
```

Once the image was pushed, it's possible to apply Statefulsets.

There is an example yaml file `./examples/wc-statefulset.yaml` for creating Statefulsets as well as persistent volumes.
In this file, replace the username `user` with your username.
This is necessary because in the Minikube VM, `/hosthome/$USER` is automatically created with your username.

```
$ sed -i "s/hosthome\/user/hosthome\/$USER/" ./examples/wc-statefulset.yaml
```

Then using thie file, create Statefulsets and persitent volumes, and watch for a moment the states of the cluster.

```
$ kubectl create -f ./examples/wc-statefulset.yaml
```

Check for pods, persistent volumes, and persistent volume claims being created.

```
$ kubectl get pods
$ kubectl get pv
$ kubectl get pvc
```

If any problem happens on a pod, investigate it:

```
$ kubectl describe pod wc-0
$ kubectl logs wc-0
```

To delete Statefulsets and persistent volumes:

```
$ kubectl delete -f ./examples/wc-statefulset.yaml
```

Note, this is just a stop-gap solution until we could make everything public through any public registry.
