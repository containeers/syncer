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

var _ = Describe("SecretSync Controller", func() {
	Context("When reconciling SecretSync", func() {
		const (
			SecretSyncName   = "test-secretsync"
			SourceNamespace  = "source-ns"
			TargetNamespace1 = "target-ns1"
			TargetNamespace2 = "target-ns2"
			SecretName       = "test-secret"
			Timeout          = time.Second * 10
			Interval         = time.Millisecond * 250
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

		It("Should successfully sync Secret to target namespaces", func() {
			By("Creating a source Secret")
			sourceSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretName,
					Namespace: SourceNamespace,
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"key1": []byte("value1"),
					"key2": []byte("value2"),
				},
			}
			Expect(k8sClient.Create(ctx, sourceSecret)).Should(Succeed())

			By("Creating a SecretSync resource")
			secretSync := &syncerv1alpha1.SecretSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretSyncName,
					Namespace: SourceNamespace,
				},
				Spec: syncerv1alpha1.SecretSyncSpec{
					SourceNamespace: SourceNamespace,
					TargetNamespaces: []string{
						TargetNamespace1,
					},
					TargetNamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"sync": "true",
						},
					},
					Secrets: []string{
						SecretName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, secretSync)).Should(Succeed())

			By("Checking if Secret is synced to target namespaces")
			for _, ns := range []string{TargetNamespace1, TargetNamespace2} {
				Eventually(func() error {
					syncedSecret := &corev1.Secret{}
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      SecretName,
						Namespace: ns,
					}, syncedSecret)
				}, Timeout, Interval).Should(Succeed())
			}

			By("Verifying Secret content in target namespaces")
			for _, ns := range []string{TargetNamespace1, TargetNamespace2} {
				syncedSecret := &corev1.Secret{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      SecretName,
					Namespace: ns,
				}, syncedSecret)).Should(Succeed())

				Expect(syncedSecret.Data).Should(Equal(sourceSecret.Data))
				Expect(syncedSecret.Type).Should(Equal(sourceSecret.Type))
			}

			By("Checking SecretSync status")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      SecretSyncName,
					Namespace: SourceNamespace,
				}, secretSync)
				if err != nil {
					return false
				}

				for _, condition := range secretSync.Status.Conditions {
					if condition.Type == "Ready" && condition.Status == "True" {
						return true
					}
				}
				return false
			}, Timeout, Interval).Should(BeTrue())
		})

		It("Should handle missing source Secret", func() {
			By("Creating a SecretSync resource with non-existent Secret")
			secretSync := &syncerv1alpha1.SecretSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretSyncName,
					Namespace: SourceNamespace,
				},
				Spec: syncerv1alpha1.SecretSyncSpec{
					SourceNamespace: SourceNamespace,
					TargetNamespaces: []string{
						TargetNamespace1,
					},
					Secrets: []string{
						"non-existent-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, secretSync)).Should(Succeed())

			By("Checking if status reflects the error")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      SecretSyncName,
					Namespace: SourceNamespace,
				}, secretSync)
				if err != nil {
					return false
				}

				for _, condition := range secretSync.Status.Conditions {
					if condition.Type == "Ready" && condition.Status == "False" {
						return true
					}
				}
				return false
			}, Timeout, Interval).Should(BeTrue())
		})

		It("Should update synced Secrets when source changes", func() {
			By("Creating initial source Secret")
			sourceSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretName,
					Namespace: SourceNamespace,
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"key1": []byte("value1"),
				},
			}
			Expect(k8sClient.Create(ctx, sourceSecret)).Should(Succeed())

			By("Creating a SecretSync resource")
			secretSync := &syncerv1alpha1.SecretSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretSyncName,
					Namespace: SourceNamespace,
				},
				Spec: syncerv1alpha1.SecretSyncSpec{
					SourceNamespace:  SourceNamespace,
					TargetNamespaces: []string{TargetNamespace1},
					Secrets:          []string{SecretName},
				},
			}
			Expect(k8sClient.Create(ctx, secretSync)).Should(Succeed())

			By("Waiting for initial sync")
			Eventually(func() error {
				syncedSecret := &corev1.Secret{}
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      SecretName,
					Namespace: TargetNamespace1,
				}, syncedSecret)
			}, Timeout, Interval).Should(Succeed())

			By("Updating source Secret")
			sourceSecret.Data["key2"] = []byte("value2")
			Expect(k8sClient.Update(ctx, sourceSecret)).Should(Succeed())

			By("Verifying the update is synced")
			Eventually(func() map[string][]byte {
				syncedSecret := &corev1.Secret{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      SecretName,
					Namespace: TargetNamespace1,
				}, syncedSecret)
				if err != nil {
					return nil
				}
				return syncedSecret.Data
			}, Timeout, Interval).Should(Equal(sourceSecret.Data))
		})
	})
})
