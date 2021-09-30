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
	"context"
	"fmt"
	"reflect"

	"github.com/submariner-io/admiral/pkg/federate"
	"github.com/submariner-io/admiral/pkg/stringset"
	"github.com/submariner-io/admiral/pkg/syncer"
	"github.com/submariner-io/admiral/pkg/util"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

func startIngressEndpointsController(svc *corev1.Service, config syncer.ResourceSyncerConfig) (*ingressEndpointsController, error) {
	var err error

	_, gvr, err := util.ToUnstructuredResource(&submarinerv1.GlobalIngressIP{}, config.RestMapper)
	if err != nil {
		return nil, err
	}

	controller := &ingressEndpointsController{
		baseSyncerController: newBaseSyncerController(),
		svcName:              svc.Name,
		namespace:            svc.Namespace,
		ingressIPMap:         stringset.NewSynchronized(),
		ingressIPs:           config.SourceClient.Resource(*gvr),
	}

	fieldSelector := fields.Set(map[string]string{"metadata.name": svc.Name}).AsSelector().String()

	controller.resourceSyncer, err = syncer.NewResourceSyncer(&syncer.ResourceSyncerConfig{
		Name:                "Ingress Endpoints syncer",
		ResourceType:        &corev1.Endpoints{},
		SourceClient:        config.SourceClient,
		SourceNamespace:     svc.Namespace,
		RestMapper:          config.RestMapper,
		Federator:           federate.NewCreateFederator(config.SourceClient, config.RestMapper, corev1.NamespaceAll),
		Scheme:              config.Scheme,
		Transform:           controller.process,
		SourceFieldSelector: fieldSelector,
		ResourcesEquivalent: areEndpointsEqual,
	})

	if err != nil {
		return nil, err
	}

	if err := controller.Start(); err != nil {
		return nil, err
	}

	ingressIPs := config.SourceClient.Resource(*gvr).Namespace(corev1.NamespaceAll)
	controller.reconcile(ingressIPs, func(obj *unstructured.Unstructured) runtime.Object {
		endpointsName, exists, _ := unstructured.NestedString(obj.Object, "spec", "serviceRef", "name")
		if exists {
			return &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      endpointsName,
					Namespace: obj.GetNamespace(),
				},
			}
		}

		return nil
	})

	klog.Infof("Created Endpoints controller for (%s/%s) with selector %q", svc.Namespace, svc.Name, fieldSelector)

	return controller, nil
}

func (c *ingressEndpointsController) process(from runtime.Object, numRequeues int, op syncer.Operation) (runtime.Object, bool) {
	endpoints := from.(*corev1.Endpoints)
	key, _ := cache.MetaNamespaceKeyFunc(endpoints)

	svcSelector := labels.SelectorFromSet(map[string]string{ServiceRefLabel: c.svcName}).String()
	gips, err := c.ingressIPs.Namespace(endpoints.Namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: svcSelector})
	if err != nil {
		return nil, false
	}

	if op == syncer.Delete {
		klog.Infof("Ingress Endpoints %s for service %s deleted", key, c.svcName)

		for i := range gips.Items {
			obj := &gips.Items[i]
			gip := &submarinerv1.GlobalIngressIP{}
			_ = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, gip)

			klog.Infof("Removing GlobalIngressIP %s for service %s deletion in namespece %s", gip.Name, endpoints.Name, endpoints.Namespace)
			c.ingressIPMap.Remove(gip.Name)
		}

		return gips, false
	}

	klog.Infof("%q ingress Endpoints %s for service %s", op, key, c.svcName)

	// Create unique list of endpoints IPs
	epIPs := map[string]bool{}
	for _, subset := range endpoints.Subsets {
		for _, addr := range subset.Addresses {
			epIPs[addr.IP] = true
		}
	}

	if len(epIPs) == 0 {
		return nil, false
	}

	c.resourceSyncer.Reconcile(func() []runtime.Object {
		ingressIPs := make([]runtime.Object, 0, len(epIPs))
		for epIP := range epIPs {
			ingressIP := &submarinerv1.GlobalIngressIP{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("ep-%.60s", epIP),
					Namespace: endpoints.Namespace,
					Labels: map[string]string{
						ServiceRefLabel: c.svcName,
					},
					Annotations: map[string]string{
						headlessSvcEndpointsIP: epIP,
					},
				},
				Spec: submarinerv1.GlobalIngressIPSpec{
					Target:     submarinerv1.HeadlessServiceEndpoints,
					ServiceRef: &corev1.LocalObjectReference{Name: c.svcName},
				},
			}

			c.ingressIPMap.Add(ingressIP.Name)
			ingressIPs = append(ingressIPs, ingressIP)
		}
		return ingressIPs
	})

	return nil, false
}

func areEndpointsEqual(obj1, obj2 *unstructured.Unstructured) bool {
	subsets1, _, _ := unstructured.NestedSlice(obj1.Object, "subsets")
	subsets2, _, _ := unstructured.NestedSlice(obj2.Object, "subsets")

	return reflect.DeepEqual(subsets1, subsets2)
}
