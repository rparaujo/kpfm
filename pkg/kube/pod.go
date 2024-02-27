package kube

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// lists the ports for all containers within a specified pod.
func ListPorts(clientset *kubernetes.Clientset, podName, namespace string) ([]string, error) {
	pod, err := clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	var ports []string
	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			ports = append(ports, fmt.Sprintf("%s:%d/%s", container.Name, port.ContainerPort, port.Protocol))
		}
	}
	return ports, nil
}
