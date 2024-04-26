package node

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/jmcgrath207/k8s-ephemeral-storage-metrics/pkg/dev"
	"github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

func (n *Node) getKubeletEndpoint(node *v1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == v1.NodeInternalIP {
			if n.kubeletReadOnlyPort > 0 {
				return "http://" + net.JoinHostPort(addr.Address, strconv.Itoa(n.kubeletReadOnlyPort))
			}
			return "https://" + net.JoinHostPort(addr.Address, strconv.Itoa(int(node.Status.DaemonEndpoints.KubeletEndpoint.Port)))
		}
	}
	return ""
}

func (n *Node) Get() {
	n.WaitGroup.Add(1)
	if n.deployType != "Deployment" {
		n.Set.Add(dev.GetEnv("CURRENT_NODE_NAME", ""))
		n.WaitGroup.Done()
		return
	}

	// Init Node slice
	startNodes, _ := dev.Clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	for _, node := range startNodes.Items {
		n.Set.Add(node.Name)
		n.KubeletEndpoint.Store(node.Name, n.getKubeletEndpoint(&node))
	}
	n.WaitGroup.Done()

}

func (n *Node) Query(node string) ([]byte, error) {
	var content []byte

	bo := backoff.NewExponentialBackOff()
	bo.MaxInterval = 1 * time.Second
	bo.MaxElapsedTime = time.Duration(n.sampleInterval) * time.Second

	operation := func() error {
		var resp *http.Response
		var err error
		if !n.scrapeFromKubelet || n.deployType != "Deployment" {
			content, err = dev.Clientset.RESTClient().Get().AbsPath(fmt.Sprintf("/api/v1/nodes/%s/proxy/stats/summary", node)).DoRaw(context.Background())
			if err != nil {
				return err
			}
		} else {
			kubeletep, ok := n.KubeletEndpoint.Load(node)
			if !ok || kubeletep == "" {
				return fmt.Errorf("kubelet endpoint not found for node: %s", node)
			}
			if n.kubeletReadOnlyPort > 0 {
				if resp, err = dev.ClientAno.Get(fmt.Sprintf("%s/stats/summary", kubeletep.(string))); err != nil {
					return err
				}
			} else {
				if resp, err = dev.ClientRaw.Get(fmt.Sprintf("%s/stats/summary", kubeletep.(string))); err != nil {
					return err
				}
			}
			defer resp.Body.Close()
			content, err = io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("failed to scrape from kubelet endpoint: unexpected status code %d: %s", resp.StatusCode, string(content))
			}
		}
		return nil
	}

	err := backoff.Retry(operation, bo)

	if err != nil {
		log.Warn().Msg(fmt.Sprintf("Failed fetched proxy stats from node : %s: %v", node, err))
		return nil, err
	}

	return content, nil

}

func (n *Node) Watch() {
	n.WaitGroup.Wait()
	stopCh := make(chan struct{})
	defer close(stopCh)
	sharedInformerFactory := informers.NewSharedInformerFactory(dev.Clientset, time.Duration(n.sampleInterval)*time.Second)
	nodeInformer := sharedInformerFactory.Core().V1().Nodes().Informer()

	// Define event handlers for Pod events
	eventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			p := obj.(*v1.Node)
			n.Set.Add(p.Name)
			n.KubeletEndpoint.Store(p.Name, n.getKubeletEndpoint(p))
		},
		DeleteFunc: func(obj interface{}) {
			p := obj.(*v1.Node)
			n.Set.Remove(p.Name)
			n.KubeletEndpoint.Delete(p.Name)
			n.evict(p.Name)
		},
	}

	// Register the event handlers with the informer
	_, err := nodeInformer.AddEventHandler(eventHandler)
	if err != nil {
		log.Err(err)
		os.Exit(1)
	}

	// Start the informer to begin watching for Node events
	go sharedInformerFactory.Start(stopCh)

	for {
		time.Sleep(time.Duration(n.sampleInterval) * time.Second)
		select {
		case <-stopCh:
			log.Error().Msg("Watcher NodeWatch stopped.")
			os.Exit(1)
		}
	}
}
