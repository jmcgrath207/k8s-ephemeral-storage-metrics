package node

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
)

var (
	AdjustedPollingRateGaugeVec *prometheus.GaugeVec
	nodeAvailableGaugeVec       *prometheus.GaugeVec
	nodeCapacityGaugeVec        *prometheus.GaugeVec
	nodePercentageGaugeVec      *prometheus.GaugeVec
)

func (n *Node) createMetrics() {

	nodeAvailableGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_node_available",
		Help: "Available ephemeral storage for a node",
	},
		[]string{
			// Name of Node where pod is placed.
			"node_name",
		},
	)

	prometheus.MustRegister(nodeAvailableGaugeVec)

	nodeCapacityGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_node_capacity",
		Help: "Capacity of ephemeral storage for a node",
	},
		[]string{
			// Name of Node where pod is placed.
			"node_name",
		},
	)

	prometheus.MustRegister(nodeCapacityGaugeVec)

	nodePercentageGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ephemeral_storage_node_percentage",
		Help: "Percentage of ephemeral storage used on a node",
	},
		[]string{
			// Name of Node where pod is placed.
			"node_name",
		},
	)

	prometheus.MustRegister(nodePercentageGaugeVec)

	if n.AdjustedPollingRate {
		AdjustedPollingRateGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "ephemeral_storage_adjusted_polling_rate",
			Help: "AdjustTime polling rate time after a Node API queries in Milliseconds",
		},
			[]string{
				// Name of Node where pod is placed.
				"node_name",
			})

		prometheus.MustRegister(AdjustedPollingRateGaugeVec)
	}

}

func (n *Node) SetMetrics(nodeName string, availableBytes float64, capacityBytes float64) {

	if n.nodeAvailable {
		nodeAvailableGaugeVec.With(prometheus.Labels{"node_name": nodeName}).Set(availableBytes)
		log.Debug().Msg(fmt.Sprintf("Node: %s availble bytes: %f", nodeName, availableBytes))
	}

	if n.nodeCapacity {
		nodeCapacityGaugeVec.With(prometheus.Labels{"node_name": nodeName}).Set(capacityBytes)
		log.Debug().Msg(fmt.Sprintf("Node: %s capacity bytes: %f", nodeName, capacityBytes))
	}

	if n.nodePercentage {
		setValue := (availableBytes / capacityBytes) * 100.0
		nodePercentageGaugeVec.With(prometheus.Labels{"node_name": nodeName}).Set(setValue)
		log.Debug().Msg(fmt.Sprintf("Node: %s percentage used: %f", nodeName, setValue))
	}

}

func (n *Node) evict(node string) {

	nodeAvailableGaugeVec.DeletePartialMatch(prometheus.Labels{"node_name": node})
	nodeCapacityGaugeVec.DeletePartialMatch(prometheus.Labels{"node_name": node})
	nodePercentageGaugeVec.DeletePartialMatch(prometheus.Labels{"node_name": node})
	if n.AdjustedPollingRate {
		AdjustedPollingRateGaugeVec.DeletePartialMatch(prometheus.Labels{"node_name": node})
	}
	log.Info().Msgf("Node %s does not exist. Removed from monitoring", node)
}
