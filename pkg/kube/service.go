package kube

import (
	"context"
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

// GetPodName returns the name of the first Pod associated with a Service.
func GetPodName(clientset *kubernetes.Clientset, namespace, serviceName string) (string, error) {
	service, err := clientset.CoreV1().Services(namespace).Get(context.Background(), serviceName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// need to handle multiple endpoints, subsets, and potential lack of endpoints.
	if len(service.Spec.Selector) == 0 {
		return "", errors.New("service has no selector")
	}

	podList, err := clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: labels.Set(service.Spec.Selector).String(),
	})
	if err != nil {
		return "", err
	}

	if len(podList.Items) == 0 {
		return "", errors.New("no pods found for this service")
	}

	// Return the name of the first Pod
	return podList.Items[0].Name, nil
}
