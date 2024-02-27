package kube

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rparaujo/kpfm/pkg/model"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/client-go/util/homedir"
)

func SetupPortForward(connection model.Connection, wg *sync.WaitGroup, statusCh chan<- model.PortForwardStatus, stopChan chan struct{}) {
	defer wg.Done()

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" && homedir.HomeDir() != "" {
		kubeconfig = filepath.Join(homedir.HomeDir(), ".kube", "config")
	}
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		statusCh <- model.PortForwardStatus{ServiceName: connection.ServiceName, Err: err}
		return
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		statusCh <- model.PortForwardStatus{ServiceName: connection.ServiceName, Err: err}
		return
	}

	// Determine the target pod name
	var podName string
	if connection.PodName != "" {
		// Use the directly specified pod name
		podName = connection.PodName
	} else if connection.ServiceName != "" {
		// Resolve the pod name from the service
		podName, err = GetPodName(clientset, connection.Namespace, connection.ServiceName)
		if err != nil {
			statusCh <- model.PortForwardStatus{ServiceName: connection.ServiceName, Err: err}
			return
		}
	} else {
		statusCh <- model.PortForwardStatus{ServiceName: connection.ServiceName, Err: fmt.Errorf("both ServiceName and PodName are empty")}
		return
	}

	roundTripper, upgrader, err := spdy.RoundTripperFor(config)
	if err != nil {
		statusCh <- model.PortForwardStatus{ServiceName: connection.ServiceName, Err: err}
		return
	}

	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", connection.Namespace, podName)
	serverURL := url.URL{
		Scheme: "https",
		Path:   path,
		Host:   strings.TrimPrefix(config.Host, "https://"),
	}

	req := clientset.CoreV1().RESTClient().
		Post().
		RequestURI(serverURL.String())

	ports := []string{fmt.Sprintf("%d:%d", connection.LocalPort, connection.RemoteServicePort)}

	logWriter := io.MultiWriter(os.Stdout)

	forwarder, err := portforward.New(
		spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, "POST", req.URL()),
		ports,
		stopChan,
		make(chan struct{}),
		logWriter,
		logWriter,
	)
	if err != nil {
		statusCh <- model.PortForwardStatus{ServiceName: connection.ServiceName, Err: err}
		return
	}

	// The forwarding is run in a separate goroutine so that it can be stopped by closing the stopChan
	go func() {
		err := forwarder.ForwardPorts()
		statusCh <- model.PortForwardStatus{ServiceName: connection.ServiceName, Err: err}
	}()
}
