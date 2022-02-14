/*
SPDX-License-Identifier: Apache-2.0

Copyright Contributors to the Submariner project.

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

package controllers

import (
	"github.com/submariner-io/admiral/pkg/syncer"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

func NewServiceExportEndpointsControllers(config *syncer.ResourceSyncerConfig) (*ServiceExportEndpointsControllers, error) {
	// We'll panic if config is nil, this is intentional
	return &ServiceExportEndpointsControllers{
		controllers: map[string]*endpointsController{},
		config:      *config,
	}, nil
}

func (c *ServiceExportEndpointsControllers) start(se *mcsv1a1.ServiceExport) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	key := c.key(se.Name, se.Namespace)
	if _, exists := c.controllers[key]; exists {
		return nil
	}

	controller, err := startEndpointsController(se.Name, se.Namespace, &c.config)
	if err != nil {
		return err
	}

	c.controllers[key] = controller

	return nil
}

func (c *ServiceExportEndpointsControllers) stopAll() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for _, controller := range c.controllers {
		controller.Stop()
	}

	c.controllers = map[string]*endpointsController{}
}

func (c *ServiceExportEndpointsControllers) stopAndCleanup(seName, seNamespace string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	key := c.key(seName, seNamespace)

	controller, exists := c.controllers[key]
	if exists {
		controller.Stop()
		delete(c.controllers, key)
	}
}

func (c *ServiceExportEndpointsControllers) key(n, ns string) string {
	return ns + "/" + n
}
