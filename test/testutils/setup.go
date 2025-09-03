package testutils

import (
	"context"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
)

func DeleteTestNamespace(ctx context.Context, k8sClient client.Client, name string) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	_ = k8sClient.Delete(ctx, ns)
}

func GetK8sClient() (client.Client, error) {

	s := runtime.NewScheme()
	if err := scheme.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("failed to add core types to scheme: %v", err)
	}
	if err := labelsv1alpha1.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("failed to add NamespaceLabel types to scheme: %v", err)
	}

	config, err := getKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %v", err)
	}

	k8sClient, err := client.New(config, client.Options{Scheme: s})
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %v", err)
	}

	return k8sClient, nil
}

func getKubeConfig() (*rest.Config, error) {

	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}

	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		kubeconfigPath = clientcmd.RecommendedHomeFile
	}

	config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %v", err)
	}

	return config, nil
}

func CleanupNamespaceLabels(ctx context.Context, k8sClient client.Client, namespace string) {
	crList := &labelsv1alpha1.NamespaceLabelList{}
	if err := k8sClient.List(ctx, crList, client.InNamespace(namespace)); err == nil {

		for _, cr := range crList.Items {
			if err := k8sClient.Delete(ctx, &cr); err != nil && !errors.IsNotFound(err) {
				fmt.Printf("Warning: failed to delete CR %s: %v\n", cr.Name, err)
			}
		}

		time.Sleep(time.Second * 2)

		crList = &labelsv1alpha1.NamespaceLabelList{}
		remainingCRs := 0
		if err := k8sClient.List(ctx, crList, client.InNamespace(namespace)); err == nil {
			remainingCRs = len(crList.Items)
		}

		if remainingCRs > 0 {
			fmt.Printf("Found %d remaining CRs, manually removing finalizers for namespace %s\n", remainingCRs, namespace)

			for _, cr := range crList.Items {

				freshCR := &labelsv1alpha1.NamespaceLabel{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, freshCR); err == nil {
					if len(freshCR.Finalizers) > 0 {
						freshCR.Finalizers = nil
						if err := k8sClient.Update(ctx, freshCR); err != nil && !errors.IsNotFound(err) {
							fmt.Printf("Warning: failed to remove finalizers from CR %s: %v\n", cr.Name, err)
						}
					}
				}
			}
		}

		Eventually(func() int {
			crList := &labelsv1alpha1.NamespaceLabelList{}
			if err := k8sClient.List(ctx, crList, client.InNamespace(namespace)); err != nil {
				return 0
			}
			return len(crList.Items)
		}, time.Second*30, time.Second).Should(Equal(0))
	}
}
