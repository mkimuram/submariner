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
	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/federate"
	"github.com/submariner-io/admiral/pkg/syncer"
	"github.com/submariner-io/admiral/pkg/util"
	"github.com/submariner-io/submariner/pkg/globalnet/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

func NewEndpointsController(config *syncer.ResourceSyncerConfig) (Interface, error) {
	// We'll panic if config is nil, this is intentional
	var err error

	klog.Info("Creating Endpoints controller")

	_, gvr, err := util.ToUnstructuredResource(&corev1.Endpoints{}, config.RestMapper)
	if err != nil {
		return nil, errors.Wrap(err, "error converting resource")
	}

	controller := &endpointsController{
		baseSyncerController: newBaseSyncerController(),
		endpoints:            config.SourceClient.Resource(*gvr),
	}

	controller.resourceSyncer, err = syncer.NewResourceSyncer(&syncer.ResourceSyncerConfig{
		Name:            "Endpoints syncer",
		ResourceType:    &corev1.Endpoints{},
		SourceClient:    config.SourceClient,
		SourceNamespace: corev1.NamespaceAll,
		RestMapper:      config.RestMapper,
		Federator:       federate.NewCreateFederator(config.SourceClient, config.RestMapper, corev1.NamespaceAll),
		Transform:       controller.process,
	})

	if err != nil {
		return nil, errors.Wrap(err, "error creating the syncer")
	}

	return controller, nil
}

func (c *endpointsController) Stop() {
	c.baseController.Stop()
}

func (c *endpointsController) Start() error {
	err := c.baseSyncerController.Start()
	if err != nil {
		return err
	}

	/*
		c.reconcile(c.endpoints, "", func(obj *unstructured.Unstructured) runtime.Object {
			ep := &corev1.Endpoints{}
			_ = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, ep)
			// Only handle endpoints with "endpoints.submariner.io/exported: true" label
			if val, ok := ep.Labels[constants.SmExportedEndpoint]; ok && val == constants.SmLabelTrue {
				return ep
			}

			return nil
		})
	*/

	return nil
}

func (c *endpointsController) process(from runtime.Object, numRequeues int, op syncer.Operation) (runtime.Object, bool) {
	ep := from.(*corev1.Endpoints)

	// Skip endpoints without "endpoints.submariner.io/exported: true" label
	if val, ok := ep.Labels[constants.SmExportedEndpoint]; !ok || val != constants.SmLabelTrue {
		return nil, false
	}

	switch op {
	case syncer.Create:
		return c.onCreate(ep)
	case syncer.Delete:
		return c.onDelete(ep)
	case syncer.Update:
	}

	return nil, false
}

func (c *endpointsController) onCreate(ep *corev1.Endpoints) (runtime.Object, bool) {
	key, _ := cache.MetaNamespaceKeyFunc(ep)

	klog.Infof("Processing Endpoints %q", key)

	clonedEp := ep.DeepCopy()
	clonedEp.ObjectMeta.Name = GetInternalSvcName(clonedEp.ObjectMeta.Name)
	delete(clonedEp.Labels, constants.SmExportedEndpoint)

	klog.Infof("Creating %#v", clonedEp)

	return clonedEp, false
}

func (c *endpointsController) onDelete(ep *corev1.Endpoints) (runtime.Object, bool) {
	key, _ := cache.MetaNamespaceKeyFunc(ep)

	klog.Infof("Endpoints %q deleted", key)

	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetInternalSvcName(ep.Name),
			Namespace: ep.Namespace,
		},
	}, false
}
