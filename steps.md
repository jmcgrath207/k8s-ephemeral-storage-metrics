

1. New minikube-docker
2. debug to deploy



```bash
[john@laptop k8s-ephemeral-storage-metrics]$ minikube ssh --node='minikube-m02' 
docker@minikube-m02:~$ sudo systemctl stop kubelet.service
```


Issue happens here when node status is not ready. Causing the node to never evict since a delete event was not received. 
```go
	content, err := Node.Query(nodeName)
	if err != nil {
		log.Warn().Msgf("Could not query node %s for ephemeral storage", nodeName)
		return
	}
```

#TODO:

Patch eviction. Follow up with e2e test automation to prevent it from regression.