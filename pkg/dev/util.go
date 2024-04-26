package dev

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	"strconv"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	Clientset *kubernetes.Clientset
	Client    *http.Client
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

	// creates the raw client
	newConfig := *config
	insecure, _ := strconv.ParseBool(GetEnv("SCRAPE_FROM_KUBELET_INSECURE", "false"))
	if insecure {
		newConfig.TLSClientConfig.Insecure = true
		newConfig.TLSClientConfig.CAFile = ""
		newConfig.TLSClientConfig.CAData = nil
	}
	if Client, err = rest.HTTPClientFor(&newConfig); err != nil {
		log.Error().Msg("Failed to get rest config for http client")
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

type LineInfoHook struct{}

func (h LineInfoHook) Run(e *zerolog.Event, l zerolog.Level, msg string) {
	_, file, line, ok := runtime.Caller(0)
	if ok {
		e.Str("line", fmt.Sprintf("%s:%d", file, line))
	}
}

func SetLogger() {
	logLevel := GetEnv("LOG_LEVEL", "info")
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		panic(err.Error())
	}
	zerolog.SetGlobalLevel(level)
	log.Hook(LineInfoHook{})

}

func EnablePprof() {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	err := http.ListenAndServe("localhost:6060", mux)
	if err != nil {
		log.Error().Msgf("Pprof could not start localhost:")
	}
}
