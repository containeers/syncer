/*
Copyright (c) 2025 Containeers.
*/

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	syncerv1alpha1 "github.com/containeers/syncer/api/v1alpha1"
)

var _ = Describe("ConfigMapSync Controller", func() {
	Context("When reconciling ConfigMapSync", func() {
		const (
			ConfigMapSyncName = "test-configmapsync"
			SourceNamespace   = "source-ns"
			TargetNamespace1  = "target-ns1"
			TargetNamespace2  = "target-ns2"
			ConfigMapName     = "test-configmap"
			Timeout           = time.Second * 10
			Interval          = time.Millisecond * 250
		)

		ctx := context.Background()

		BeforeEach(func() {
			// Create source namespace
			sourceNS := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: SourceNamespace,
				},
			}
			Expect(k8sClient.Create(ctx, sourceNS)).Should(Succeed())

			// Create target namespaces
			targetNS1 := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: TargetNamespace1,
					Labels: map[string]string{
						"sync": "true",
					},
				},
			}
			Expect(k8sClient.Create(ctx, targetNS1)).Should(Succeed())

			targetNS2 := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: TargetNamespace2,
					Labels: map[string]string{
						"sync": "true",
					},
				},
			}
			Expect(k8sClient.Create(ctx, targetNS2)).Should(Succeed())
		})

		AfterEach(func() {
			// Cleanup
			Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: SourceNamespace}})).Should(Succeed())
			Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: TargetNamespace1}})).Should(Succeed())
			Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: TargetNamespace2}})).Should(Succeed())
		})

		It("Should successfully sync ConfigMap to target namespaces", func() {
			By("Creating a source ConfigMap")
			sourceConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ConfigMapName,
					Namespace: SourceNamespace,
				},
				Data: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			}
			Expect(k8sClient.Create(ctx, sourceConfigMap)).Should(Succeed())

			By("Creating a ConfigMapSync resource")
			configMapSync := &syncerv1alpha1.ConfigMapSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ConfigMapSyncName,
					Namespace: SourceNamespace,
				},
				Spec: syncerv1alpha1.ConfigMapSyncSpec{
					SourceNamespace: SourceNamespace,
					TargetNamespaces: []string{
						TargetNamespace1,
					},
					TargetNamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"sync": "true",
						},
					},
					ConfigMaps: []string{
						ConfigMapName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, configMapSync)).Should(Succeed())

			By("Checking if ConfigMap is synced to target namespaces")
			for _, ns := range []string{TargetNamespace1, TargetNamespace2} {
				Eventually(func() error {
					syncedConfigMap := &corev1.ConfigMap{}
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      ConfigMapName,
						Namespace: ns,
					}, syncedConfigMap)
				}, Timeout, Interval).Should(Succeed())
			}

			By("Verifying ConfigMap content in target namespaces")
			for _, ns := range []string{TargetNamespace1, TargetNamespace2} {
				syncedConfigMap := &corev1.ConfigMap{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      ConfigMapName,
					Namespace: ns,
				}, syncedConfigMap)).Should(Succeed())

				Expect(syncedConfigMap.Data).Should(Equal(sourceConfigMap.Data))
			}

			By("Checking ConfigMapSync status")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      ConfigMapSyncName,
					Namespace: SourceNamespace,
				}, configMapSync)
				if err != nil {
					return false
				}

				// Check if status conditions indicate success
				for _, condition := range configMapSync.Status.Conditions {
					if condition.Type == "Ready" && condition.Status == "True" {
						return true
					}
				}
				return false
			}, Timeout, Interval).Should(BeTrue())
		})

		It("Should handle missing source ConfigMap", func() {
			By("Creating a ConfigMapSync resource with non-existent ConfigMap")
			configMapSync := &syncerv1alpha1.ConfigMapSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ConfigMapSyncName,
					Namespace: SourceNamespace,
				},
				Spec: syncerv1alpha1.ConfigMapSyncSpec{
					SourceNamespace: SourceNamespace,
					TargetNamespaces: []string{
						TargetNamespace1,
					},
					ConfigMaps: []string{
						"non-existent-configmap",
					},
				},
			}
			Expect(k8sClient.Create(ctx, configMapSync)).Should(Succeed())

			By("Checking if status reflects the error")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      ConfigMapSyncName,
					Namespace: SourceNamespace,
				}, configMapSync)
				if err != nil {
					return false
				}

				for _, condition := range configMapSync.Status.Conditions {
					if condition.Type == "Ready" && condition.Status == "False" {
						return true
					}
				}
				return false
			}, Timeout, Interval).Should(BeTrue())
		})

		It("Should update synced ConfigMaps when source changes", func() {
			By("Creating initial source ConfigMap")
			sourceConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ConfigMapName,
					Namespace: SourceNamespace,
				},
				Data: map[string]string{
					"key1": "value1",
				},
			}
			Expect(k8sClient.Create(ctx, sourceConfigMap)).Should(Succeed())

			By("Creating a ConfigMapSync resource")
			configMapSync := &syncerv1alpha1.ConfigMapSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ConfigMapSyncName,
					Namespace: SourceNamespace,
				},
				Spec: syncerv1alpha1.ConfigMapSyncSpec{
					SourceNamespace:  SourceNamespace,
					TargetNamespaces: []string{TargetNamespace1},
					ConfigMaps:       []string{ConfigMapName},
				},
			}
			Expect(k8sClient.Create(ctx, configMapSync)).Should(Succeed())

			By("Waiting for initial sync")
			Eventually(func() error {
				syncedConfigMap := &corev1.ConfigMap{}
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      ConfigMapName,
					Namespace: TargetNamespace1,
				}, syncedConfigMap)
			}, Timeout, Interval).Should(Succeed())

			By("Updating source ConfigMap")
			sourceConfigMap.Data["key2"] = "value2"
			Expect(k8sClient.Update(ctx, sourceConfigMap)).Should(Succeed())

			By("Verifying the update is synced")
			Eventually(func() map[string]string {
				syncedConfigMap := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      ConfigMapName,
					Namespace: TargetNamespace1,
				}, syncedConfigMap)
				if err != nil {
					return nil
				}
				return syncedConfigMap.Data
			}, Timeout, Interval).Should(Equal(sourceConfigMap.Data))
		})
	})
})
