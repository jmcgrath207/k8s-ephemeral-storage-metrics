package pod

import (
	"context"
	"github.com/jmcgrath207/k8s-ephemeral-storage-metrics/pkg/dev"
	"github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"os"
	"time"
)

type pod struct {
	containers []container
}

type container struct {
	name            string
	limit           float64
	emptyDirVolumes []emptyDirVolumes
}

type emptyDirVolumes struct {
	name      string
	mountPath string
	sizeLimit float64
}

// Collector for pod data
func (cr Collector) getPodData(p v1.Pod) {
	if p.Status.Phase == "Running" {
		var collectContainers []container

		for _, x := range p.Spec.Containers {
			collectContainers = append(collectContainers, cr.getContainerData(x, p))
		}

		cr.lookupMutex.Lock()
		(*cr.lookup)[p.Name] = pod{containers: collectContainers}
		cr.lookupMutex.Unlock()
	}
}

func (cr Collector) initGetPodsData() {
	// Init Get List of all pods
	pods, err := dev.Clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Error().Msgf("Error getting pods: %v\n", err)
		os.Exit(1)
	}

	for _, p := range pods.Items {
		cr.getPodData(p)
	}
	cr.WaitGroup.Done()

}

func (cr Collector) podWatch() {
	cr.WaitGroup.Wait()
	stopCh := make(chan struct{})
	defer close(stopCh)
	sharedInformerFactory := informers.NewSharedInformerFactory(dev.Clientset, time.Duration(cr.sampleInterval)*time.Second)
	podInformer := sharedInformerFactory.Core().V1().Pods().Informer()

	// Define event handlers for Pod events
	eventHandler := cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, newObj interface{}) {
			p := newObj.(*v1.Pod)
			cr.getPodData(*p)
		},
		DeleteFunc: func(obj interface{}) {
			p := obj.(*v1.Pod)
			cr.lookupMutex.Lock()
			delete(*cr.lookup, p.Name)
			cr.lookupMutex.Unlock()
			evictPodFromMetrics(*p)
		},
	}

	// Register the event handlers with the informer
	_, err := podInformer.AddEventHandler(eventHandler)
	if err != nil {
		log.Err(err)
		os.Exit(1)
	}

	// Start the informer to begin watching for Pod events
	go sharedInformerFactory.Start(stopCh)

	for {
		time.Sleep(time.Duration(cr.sampleInterval) * time.Second)
		select {
		case <-stopCh:
			log.Error().Msg("Watcher podWatch stopped.")
			os.Exit(1)
		}
	}

}

// Collector for container data
func (cr Collector) getContainerData(c v1.Container, p v1.Pod) container {

	setContainer := container{}
	setContainer.name = c.Name
	matchKey := v1.ResourceName("ephemeral-storage")

	if cr.containerVolumeUsage && cr.containerVolumeLimitsPercentage && p.Spec.Volumes != nil {
		collectMounts := false

		podMountsMap := make(map[string]float64)
		for _, v := range p.Spec.Volumes {
			if v.VolumeSource.EmptyDir != nil {
				podMountsMap[v.Name] = 0
				collectMounts = true
				if v.VolumeSource.EmptyDir.SizeLimit != nil {
					podMountsMap[v.Name] = v.VolumeSource.EmptyDir.SizeLimit.AsApproximateFloat64()
					collectMounts = true
				}
			}
		}

		if collectMounts {
			var collectVolumes []emptyDirVolumes
			for _, volumeMount := range c.VolumeMounts {
				size, ok := podMountsMap[volumeMount.Name]
				if ok {
					collectVolumes = append(collectVolumes, emptyDirVolumes{name: volumeMount.Name, mountPath: volumeMount.MountPath, sizeLimit: size})
				}
			}

			setContainer.emptyDirVolumes = collectVolumes

		}

	}
	if cr.containerLimitsPercentage {
		for key, val := range c.Resources.Limits {
			if key == matchKey {
				setContainer.limit = val.AsApproximateFloat64()
			}
		}
	}
	return setContainer
}
