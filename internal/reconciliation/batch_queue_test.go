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

package reconciliation

import (
	"time"

	agentoctopuscomv1beta1 "github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("BatchQueue", func() {
	var (
		queue     BatchQueue
		testBatch *Batch
	)

	BeforeEach(func() {
		queue = NewBatchQueue()

		// Create a test batch with mock WSA resource
		testWSA := &agentoctopuscomv1beta1.WorkloadServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-wsa",
				Namespace: "default",
			},
		}
		wsaResource := rules.NewWSAResource(testWSA)

		testBatch = &Batch{
			ID:        NewBatchID(),
			Resources: []rules.WSAResource{wsaResource},
			StartTime: time.Now(),
		}
	})

	AfterEach(func() {
		queue.ShutDown()
	})

	Describe("NewBatchQueue", func() {
		It("should create a new batch queue", func() {
			newQueue := NewBatchQueue()
			Expect(newQueue).NotTo(BeNil())
		})
	})

	Describe("Add and Get operations", func() {
		Context("when adding and getting a batch", func() {
			It("should store and retrieve the batch correctly", func() {
				// Add batch to queue
				queue.Add(testBatch)

				// Get batch from queue
				retrievedBatch, shutdown := queue.Get()
				Expect(shutdown).To(BeFalse())
				Expect(retrievedBatch).NotTo(BeNil())
				Expect(retrievedBatch.ID).To(Equal(testBatch.ID))
			})
		})

		Context("when queue is empty", func() {
			It("should block on Get until item is added", func() {
				done := make(chan struct{})
				var retrievedBatch *Batch
				var shutdown bool

				// Start goroutine to get from empty queue
				go func() {
					defer GinkgoRecover()
					retrievedBatch, shutdown = queue.Get()
					close(done)
				}()

				// Verify it's blocking
				Consistently(done).ShouldNot(BeClosed())

				// Add item to unblock
				queue.Add(testBatch)

				// Now it should complete
				Eventually(done).Should(BeClosed())
				Expect(shutdown).To(BeFalse())
				Expect(retrievedBatch).NotTo(BeNil())
				Expect(retrievedBatch.ID).To(Equal(testBatch.ID))
			})
		})

		Context("when adding multiple batches", func() {
			It("should handle FIFO ordering", func() {
				// Create multiple test batches
				batch1 := &Batch{ID: NewBatchID(), Resources: []rules.WSAResource{}, StartTime: time.Now()}
				batch2 := &Batch{ID: NewBatchID(), Resources: []rules.WSAResource{}, StartTime: time.Now()}
				batch3 := &Batch{ID: NewBatchID(), Resources: []rules.WSAResource{}, StartTime: time.Now()}

				// Add batches in order
				queue.Add(batch1)
				queue.Add(batch2)
				queue.Add(batch3)

				// Retrieve batches and verify order
				retrieved1, _ := queue.Get()
				retrieved2, _ := queue.Get()
				retrieved3, _ := queue.Get()

				Expect(retrieved1.ID).To(Equal(batch1.ID))
				Expect(retrieved2.ID).To(Equal(batch2.ID))
				Expect(retrieved3.ID).To(Equal(batch3.ID))
			})
		})
	})

	Describe("AddRateLimited operations", func() {
		It("should add batch with rate limiting", func() {
			queue.AddRateLimited(testBatch)

			retrievedBatch, shutdown := queue.Get()
			Expect(shutdown).To(BeFalse())
			Expect(retrievedBatch).NotTo(BeNil())
			Expect(retrievedBatch.ID).To(Equal(testBatch.ID))
		})

		It("should track requeue count", func() {
			// Add batch with rate limiting multiple times
			queue.AddRateLimited(testBatch)
			queue.Done(testBatch)
			queue.AddRateLimited(testBatch)
			queue.Done(testBatch)

			// Check requeue count
			count := queue.NumRequeues(testBatch)
			Expect(count).To(BeNumerically(">=", 1))
		})
	})

	Describe("Done and Forget operations", func() {
		BeforeEach(func() {
			queue.Add(testBatch)
		})

		It("should mark batch as done", func() {
			retrievedBatch, _ := queue.Get()
			Expect(retrievedBatch.ID).To(Equal(testBatch.ID))

			// Mark as done - should not panic
			Expect(func() {
				queue.Done(testBatch)
			}).NotTo(Panic())
		})

		It("should forget batch and reset requeue count", func() {
			// Add and requeue multiple times to build up count
			queue.AddRateLimited(testBatch)
			queue.Done(testBatch)
			queue.AddRateLimited(testBatch)
			queue.Done(testBatch)

			initialCount := queue.NumRequeues(testBatch)
			Expect(initialCount).To(BeNumerically(">", 0))

			// Forget the batch
			queue.Forget(testBatch)

			// Requeue count should be reset
			newCount := queue.NumRequeues(testBatch)
			Expect(newCount).To(Equal(0))
		})
	})

	Describe("NumRequeues", func() {
		It("should return 0 for new batch", func() {
			count := queue.NumRequeues(testBatch)
			Expect(count).To(Equal(0))
		})

		It("should track requeue count correctly", func() {
			// Add with rate limiting multiple times
			for i := 0; i < 3; i++ {
				queue.AddRateLimited(testBatch)
				queue.Done(testBatch)
			}

			count := queue.NumRequeues(testBatch)
			Expect(count).To(BeNumerically(">=", 2))
		})
	})

	Describe("ShutDown", func() {
		It("should shut down the queue gracefully", func() {
			// Shut down the queue first
			queue.ShutDown()

			// Get should return shutdown=true
			_, shutdown := queue.Get()
			Expect(shutdown).To(BeTrue())
		})

		It("should handle multiple shutdown calls", func() {
			Expect(func() {
				queue.ShutDown()
				queue.ShutDown() // Second call should not panic
			}).NotTo(Panic())
		})

		It("should return shutdown immediately after shutdown", func() {
			queue.ShutDown()

			// Multiple Get calls should return shutdown=true
			_, shutdown1 := queue.Get()
			_, shutdown2 := queue.Get()

			Expect(shutdown1).To(BeTrue())
			Expect(shutdown2).To(BeTrue())
		})
	})

	Describe("Concurrent operations", func() {
		It("should handle concurrent Add and Get operations", func() {
			const numGoroutines = 10
			const batchesPerGoroutine = 5

			resultChan := make(chan *Batch, numGoroutines*batchesPerGoroutine)

			// Start goroutines to add batches
			for i := 0; i < numGoroutines; i++ {
				go func() {
					defer GinkgoRecover()
					for j := 0; j < batchesPerGoroutine; j++ {
						batch := &Batch{
							ID:        NewBatchID(),
							Resources: []rules.WSAResource{},
							StartTime: time.Now(),
						}
						queue.Add(batch)
					}
				}()
			}

			// Start goroutines to get batches
			for i := 0; i < numGoroutines; i++ {
				go func() {
					defer GinkgoRecover()
					for j := 0; j < batchesPerGoroutine; j++ {
						batch, shutdown := queue.Get()
						if !shutdown && batch != nil {
							resultChan <- batch
							queue.Done(batch)
						}
					}
				}()
			}

			// Collect results
			var results []*Batch
			timeout := time.After(5 * time.Second)
			for len(results) < numGoroutines*batchesPerGoroutine {
				select {
				case batch := <-resultChan:
					results = append(results, batch)
				case <-timeout:
					Fail("Timeout waiting for concurrent operations to complete")
				}
			}

			Expect(results).To(HaveLen(numGoroutines * batchesPerGoroutine))

			// Verify all batch IDs are unique
			seenIDs := make(map[BatchID]bool)
			for _, batch := range results {
				Expect(seenIDs[batch.ID]).To(BeFalse(), "Duplicate batch ID found: %s", batch.ID)
				seenIDs[batch.ID] = true
			}
		})

		It("should handle concurrent Done and Forget operations", func() {
			const numOperations = 50

			// Add the same batch multiple times
			for i := 0; i < numOperations; i++ {
				queue.AddRateLimited(testBatch)
				queue.Done(testBatch)
			}

			done := make(chan struct{})

			// Concurrent Done operations
			go func() {
				defer GinkgoRecover()
				for i := 0; i < numOperations/2; i++ {
					queue.Done(testBatch)
				}
				close(done)
			}()

			// Concurrent Forget operations
			go func() {
				defer GinkgoRecover()
				for i := 0; i < numOperations/2; i++ {
					queue.Forget(testBatch)
				}
			}()

			Eventually(done).Should(BeClosed())

			// Should not panic and queue should still be functional
			queue.Add(testBatch)
			retrievedBatch, shutdown := queue.Get()
			Expect(shutdown).To(BeFalse())
			Expect(retrievedBatch).NotTo(BeNil())
		})
	})

	Describe("Edge cases", func() {
		It("should handle nil batch gracefully", func() {
			// These operations should not panic with nil batch
			Expect(func() {
				queue.Done(nil)
				queue.Forget(nil)
				count := queue.NumRequeues(nil)
				Expect(count).To(Equal(0))
			}).NotTo(Panic())
		})

		It("should handle rapid Add/Get cycles", func() {
			const numCycles = 100

			for i := 0; i < numCycles; i++ {
				batch := &Batch{
					ID:        NewBatchID(),
					Resources: []rules.WSAResource{},
					StartTime: time.Now(),
				}

				queue.Add(batch)
				retrievedBatch, shutdown := queue.Get()

				Expect(shutdown).To(BeFalse())
				Expect(retrievedBatch).NotTo(BeNil())
				Expect(retrievedBatch.ID).To(Equal(batch.ID))

				queue.Done(batch)
			}
		})
	})
})
