package pod

import (
	"testing"
)

func TestGetPodsListOptions(t *testing.T) {
	tests := []struct {
		name              string
		listPodsWithCache bool
		deployAsDaemonSet bool
		currentNodeName   string
		wantResourceVer   string
		wantFieldSelector string
		wantLimit         int64
	}{
		{
			name:              "defaults",
			listPodsWithCache: false,
			deployAsDaemonSet: false,
			currentNodeName:   "",
			wantResourceVer:   "",
			wantFieldSelector: "",
			wantLimit:         500,
		},
		{
			name:              "cache only",
			listPodsWithCache: true,
			deployAsDaemonSet: false,
			currentNodeName:   "",
			wantResourceVer:   "0",
			wantFieldSelector: "",
			wantLimit:         500,
		},
		{
			name:              "daemonset only",
			listPodsWithCache: false,
			deployAsDaemonSet: true,
			currentNodeName:   "node-1",
			wantResourceVer:   "",
			wantFieldSelector: "spec.nodeName=node-1",
			wantLimit:         500,
		},
		{
			name:              "cache + daemonset",
			listPodsWithCache: true,
			deployAsDaemonSet: true,
			currentNodeName:   "node-2",
			wantResourceVer:   "0",
			wantFieldSelector: "spec.nodeName=node-2",
			wantLimit:         500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Collector{
				listPodsWithCache: tt.listPodsWithCache,
				deployAsDaemonSet: tt.deployAsDaemonSet,
				currentNodeName:   tt.currentNodeName,
			}
			opts := c.getPodsListOptions()
			if opts.ResourceVersion != tt.wantResourceVer {
				t.Errorf("ResourceVersion = %q, want %q", opts.ResourceVersion, tt.wantResourceVer)
			}
			if opts.FieldSelector != tt.wantFieldSelector {
				t.Errorf("FieldSelector = %q, want %q", opts.FieldSelector, tt.wantFieldSelector)
			}
			if opts.Limit != tt.wantLimit {
				t.Errorf("Limit = %d, want %d", opts.Limit, tt.wantLimit)
			}
		})
	}
}
