<<<<<<< HEAD
package controller

import (
	. "github.com/onsi/ginkgo/v2"
=======
/*
Copyright 2025.

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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentoctopuscomv1beta1 "github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/reconciliation"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
>>>>>>> tmp-original-05-05-26-00-18
)

var _ = Describe("WorkloadServiceAccount Controller", func() {
	Context("When reconciling a resource", func() {

		It("should successfully reconcile the resource", func() {
<<<<<<< HEAD
=======
			By("Reconciling the created resource")
			engine := rules.NewInMemoryEngine(k8sClient, scheme.Scheme, targetNamespaceRegex, 5*time.Minute)
			eventCollector := reconciliation.NewEventCollector(500*time.Millisecond, 100)
			controllerReconciler := &WorkloadServiceAccountReconciler{
				Client:         k8sClient,
				Scheme:         k8sClient.Scheme(),
				Engine:         &engine,
				EventCollector: eventCollector,
			}
>>>>>>> tmp-original-05-05-26-00-18

			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
