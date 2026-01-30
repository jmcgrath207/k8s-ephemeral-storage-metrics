package env

import "github.com/jmcgrath207/k8s-ephemeral-storage-metrics/pkg/dev"

func DeployAsDaemonSet() bool {
	return dev.GetEnv("DEPLOY_TYPE", "DaemonSet") == "DaemonSet"
}

func CurrentNodeName() string {
	return dev.GetEnv("CURRENT_NODE_NAME", "")
}
