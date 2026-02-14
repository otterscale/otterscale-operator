/*
Copyright 2026 The OtterScale Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tenant

import (
	"sync/atomic"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

// IstioDetector provides a thread-safe way to check whether Istio CRDs are available
// in the cluster. It uses an atomic boolean that is refreshed via the Discovery API
// whenever a relevant CRD event is observed.
type IstioDetector struct {
	config  *rest.Config
	enabled atomic.Bool
}

// NewIstioDetector creates an IstioDetector and performs an initial availability check.
func NewIstioDetector(config *rest.Config) *IstioDetector {
	d := &IstioDetector{config: config}
	d.Refresh()
	return d
}

// IsEnabled returns whether Istio CRDs are currently available in the cluster.
func (d *IstioDetector) IsEnabled() bool {
	return d.enabled.Load()
}

// Refresh re-checks Istio CRD availability via the Discovery API and updates the flag.
// On error the previous state is preserved; this is intentional to avoid flapping
// caused by transient API-server issues.
func (d *IstioDetector) Refresh() {
	enabled, err := checkIstioEnabled(d.config)
	if err != nil {
		// Keep the previous state on transient errors.
		return
	}
	d.enabled.Store(enabled)
}

// checkIstioEnabled checks if Istio is installed in the cluster.
func checkIstioEnabled(c *rest.Config) (bool, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(c)
	if err != nil {
		return false, err
	}
	return isResourceSupported(dc, "security.istio.io/v1"), nil
}

// isResourceSupported checks if a CRD exists in the cluster.
// This allows the controller to adapt its behavior based on the environment.
func isResourceSupported(dc *discovery.DiscoveryClient, groupVersion string) bool {
	if _, err := dc.ServerResourcesForGroupVersion(groupVersion); err != nil {
		return false
	}
	return true
}
