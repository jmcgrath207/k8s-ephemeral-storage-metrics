package dev

import (
	"github.com/rs/zerolog/log"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"os"
)

var (
	Clientset *kubernetes.Clientset
)

func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func SetK8sClient() {

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Error().Msg("Failed to get rest config for in cluster client")
		panic(err.Error())
	}
	// creates the clientset
	Clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		log.Error().Msg("Failed to get client set for in cluster client")
		panic(err.Error())
	}
	log.Debug().Msg("Successful got the in cluster client")

}
