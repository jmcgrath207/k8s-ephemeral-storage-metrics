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
	ClientRaw *http.Client
	ClientAno *http.Client
)

func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func setScrapeFromKubelet(config *rest.Config) {

	var err error

	// creates the raw client with authentication
	newConfig := *config
	insecure, _ := strconv.ParseBool(GetEnv("SCRAPE_FROM_KUBELET_TLS_INSECURE_SKIP_VERIFY", "false"))
	if insecure {
		newConfig.TLSClientConfig.Insecure = true
		newConfig.TLSClientConfig.CAFile = ""
		newConfig.TLSClientConfig.CAData = nil
	}
	if ClientRaw, err = rest.HTTPClientFor(&newConfig); err != nil {
		log.Error().Msg("Failed to get raw http client")
		panic(err.Error())
	}

	// creates the raw client without authentication
	anoConfig := rest.AnonymousClientConfig(config)
	if ClientAno, err = rest.HTTPClientFor(anoConfig); err != nil {
		log.Error().Msg("Failed to get anonymous http client")
		panic(err.Error())
	}
}

func SetK8sClient() {

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Error().Msg("Failed to get rest config for in cluster client")
		panic(err.Error())
	}

	scrapeFromKubelet, _ := strconv.ParseBool(GetEnv("SCRAPE_FROM_KUBELET", "false"))
	if scrapeFromKubelet {
		setScrapeFromKubelet(config)
	}

	// fix: reading ops and burst from os env.
	if qps, err := strconv.ParseFloat(GetEnv("CLIENT_GO_QPS", "5"), 32); err == nil {
		config.QPS = float32(qps)
	}
	if burst, err := strconv.Atoi(GetEnv("CLIENT_GO_BURST", "10")); err == nil {
		config.Burst = burst
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
