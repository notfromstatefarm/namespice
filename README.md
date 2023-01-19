# namespice
### Sprinkle some spice on your Kubernetes namespaces

Do you find yourself repeating identical resources between namespaces? **namespice** is a Kubernetes controller that
helps you DRY things up and centrally manage these resources via sets of `NamespaceClass` CRDs.

## Usage

1. Install namespice CRD and controller.
2. Create a `NamespaceClass`
3. Annotate your namespace
4. Resources are managed for you!

The controller will watch for changes and instantly create and delete resources as needed.

### Quick install (testing only)
`curl https://raw.githubusercontent.com/notfromstatefarm/namespice/main/manifests/install.yaml | kubectl apply -f -`

Note that with the default manifests namespice will have full unrestricted access to your cluster. Do not use this in production.

### Install just the CRDs

`curl https://raw.githubusercontent.com/notfromstatefarm/namespice/main/manifests/crd.yaml | kubectl apply -f -`

### NamespaceClass example
```yaml
apiVersion: namespice.io/v1
kind: NamespaceClass
metadata:
  name: example-class
resources:
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: example-cm
    data:
      lorem: "ipsum"
```

### Namespace annotation
```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: example-ns
  annotations:
    namespice.io/class: example-class
```
You can also specify multiple classes for a namespace, separating them with commas.

## Extended Example
In this example, we create two namespaces `english` and `spanish`. In each namespace, we want to have a deployment constantly printing
"Hello World" in a certain language. To accomplish this, we define three `NamespaceClass`. Two will contain a `ConfigMap` providing
an environment variable `GREETING`, and the third will define the common deployment between the two namespaces that consumes the `ConfigMap`.

```yaml
apiVersion: namespice.io/v1
kind: NamespaceClass
metadata:
  name: lang-english
resources:
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: language
    data:
      greeting: "hello world!"
---
apiVersion: namespice.io/v1
kind: NamespaceClass
metadata:
  name: lang-spanish
resources:
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: language
    data:
      greeting: "hola mundo!"
---
apiVersion: namespice.io/v1
kind: NamespaceClass
metadata:
  name: lang-common
resources:
  - apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: greeter
      labels:
        app: greeter
    spec:
      replicas: 1
      selector:
        matchLabels:
          app: greeter
      template:
        metadata:
          labels:
            app: greeter
        spec:
          containers:
            - name: greeter
              image: busybox
              command:
                - sh
                - -c
                - "while true; do echo $GREETING; sleep 2; done"
              env:
                - name: GREETING
                  valueFrom:
                    configMapKeyRef:
                      name: language
                      key: greeting
---
apiVersion: v1
kind: Namespace
metadata:
  name: english
  annotations:
    namespice.io/class: lang-common,lang-english
---
apiVersion: v1
kind: Namespace
metadata:
  name: spanish
  annotations:
    namespice.io/class: lang-common,lang-spanish
```

## Limitations
* Updates to already created resources is not yet implemented, i.e. changing an entry of a `ConfigMap` inside of a `NamespaceClass` will not propagate to the existing resources.