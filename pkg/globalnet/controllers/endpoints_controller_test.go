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

package controllers_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/syncer"
	"github.com/submariner-io/submariner/pkg/globalnet/controllers"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("Endpoints controller", func() {
	t := newEndpointsControllerTestDriver()

	var (
		epName string
		ep     *corev1.Endpoints
	)

	When("an endpoints without label is created", func() {
		BeforeEach(func() {
			epName = "epWithoutLabel"
			t.createEndpoints(newEndpoints(epName, map[string]string{}))
			t.awaitEndpoints(epName)
		})

		It("should not create a cloned endpoints", func() {
			t.awaitNoEndpoints(controllers.GetInternalSvcName(epName))
		})
	})

	When("an endpoints with label is created", func() {
		BeforeEach(func() {
			epName = "epWithLabel"
			ep = t.createEndpoints(newEndpoints(epName, map[string]string{"endpoints.submariner.io/exported": "true"}))
			t.awaitEndpoints(epName)
		})

		It("should create a cloned endpoints and delete it once original one is deleted", func() {
			clonedEp := t.awaitEndpoints(controllers.GetInternalSvcName(epName))
			t.deleteEndpoints(ep)
			t.awaitNoEndpoints(epName)
			t.awaitNoEndpoints(clonedEp.Name)
		})
	})
})

type endpointsControllerTestDriver struct {
	*testDriverBase
}

func newEndpointsControllerTestDriver() *endpointsControllerTestDriver {
	t := &endpointsControllerTestDriver{}

	BeforeEach(func() {
		t.testDriverBase = newTestDriverBase()
	})

	JustBeforeEach(func() {
		t.start()
	})

	AfterEach(func() {
		t.testDriverBase.afterEach()
	})

	return t
}

func (t *endpointsControllerTestDriver) start() {
	var err error

	config := &syncer.ResourceSyncerConfig{
		SourceClient: t.dynClient,
		RestMapper:   t.restMapper,
		Scheme:       t.scheme,
	}

	t.controller, err = controllers.NewEndpointsController(config)

	Expect(err).To(Succeed())
	Expect(t.controller.Start()).To(Succeed())
}
