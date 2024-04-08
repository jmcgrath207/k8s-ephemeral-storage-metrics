package node

import (
	"context"
	"fmt"
	"github.com/cenkalti/backoff/v4"
	"github.com/jmcgrath207/k8s-ephemeral-storage-metrics/pkg/dev"
	"github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"os"
	"time"
)

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
	}
	n.WaitGroup.Done()

}

func (n *Node) Query(node string) ([]byte, error) {
	var content []byte

	bo := backoff.NewExponentialBackOff()
	bo.MaxInterval = 1 * time.Second
	bo.MaxElapsedTime = time.Duration(n.sampleInterval) * time.Second

	operation := func() error {
		var err error
		content, err = dev.Clientset.RESTClient().Get().AbsPath(fmt.Sprintf("/api/v1/nodes/%s/proxy/stats/summary", node)).DoRaw(context.Background())
		if err != nil {
			return err
		}
		return nil
	}

	err := backoff.Retry(operation, bo)

	if err != nil {
		log.Warn().Msg(fmt.Sprintf("Failed fetched proxy stats from node : %s", node))
		return nil, err
	}

	return content, nil

}

func (n *Node) Watch() {
	n.WaitGroup.Wait()
	stopCh := make(chan struct{})
	defer close(stopCh)
	sharedInformerFactory := informers.NewSharedInformerFactory(dev.Clientset, time.Duration(n.sampleInterval)*time.Second)
	podInformer := sharedInformerFactory.Core().V1().Nodes().Informer()

	// Define event handlers for Pod events
	eventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			p := obj.(*v1.Node)
			n.Set.Add(p.Name)
		},
		DeleteFunc: func(obj interface{}) {
			p := obj.(*v1.Node)
			n.Set.Remove(p.Name)
			n.evict(p.Name)
		},
	}

	// Register the event handlers with the informer
	_, err := podInformer.AddEventHandler(eventHandler)
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
