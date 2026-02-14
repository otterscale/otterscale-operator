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
	"context"
	"errors"
	"time"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type IstioPoller struct {
	Config   *rest.Config
	Interval time.Duration
}

func (p *IstioPoller) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("IstioPoller")

	logger.Info("Starting Istio detection poller...")

	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Stopping Istio poller")
			return nil

		case <-ticker.C:
			enabled, err := checkIstioEnabled(p.Config)
			if err != nil {
				logger.Error(err, "Error checking Istio status")
				continue
			}
			if enabled {
				logger.Info("Istio detected via polling! Restarting operator...")
				return errors.New("istio detected, restarting operator")
			}
		}
	}
}

// checkIstioEnabled checks if Istio is installed in the cluster.
func checkIstioEnabled(c *rest.Config) (bool, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(c)
	if err != nil {
		return false, err
	}
	return isResourceSupported(dc, "networking.istio.io/v1beta1"), nil
}

// isResourceSupported checks if a CRD exists in the cluster.
// This allows the controller to adapt its behavior based on the environment.
func isResourceSupported(dc *discovery.DiscoveryClient, groupVersion string) bool {
	if _, err := dc.ServerResourcesForGroupVersion(groupVersion); err != nil {
		return false
	}
	return true
}
