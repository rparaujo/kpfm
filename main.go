package main

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/client-go/util/homedir"
)

type Config struct {
	Profiles map[string]Profile `yaml:"profiles"`
}

type Profile struct {
	Services []Service `yaml:"services"`
}

type Service struct {
	Name       string `yaml:"name"`
	Namespace  string `yaml:"namespace"`
	LocalPort  int    `yaml:"localPort"`
	RemotePort int    `yaml:"remotePort"`
}

func readConfig(filename string) (*Config, error) {
	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	c := &Config{}
	err = yaml.Unmarshal(buf, c)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func getPodNameFromService(clientset *kubernetes.Clientset, namespace, serviceName string) (string, error) {
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

func setupPortForward(service Service, wg *sync.WaitGroup) {
	defer wg.Done()

	// Load kubeconfig and create a clientset
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" && homedir.HomeDir() != "" {
		kubeconfig = filepath.Join(homedir.HomeDir(), ".kube", "config")
	}
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig) // Use default kubeconfig path
	if err != nil {
		log.Fatalf("Error building kubeconfig: %s", err)
	}

	// Create Kubernetes client
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating Kubernetes client: %s", err)
	}

	// Create a SPDY roundtripper to handle upgrade requests
	roundTripper, upgrader, err := spdy.RoundTripperFor(config)
	if err != nil {
		log.Fatalf("Error creating round tripper: %s", err)
	}

	// This approach requires additional logic to handle scenarios where a service routes to multiple pods or when there are no pods available.
	// (e.g., load balancing logic, failover mechanisms).
	podName, err := getPodNameFromService(clientset, service.Namespace, service.Name)
	if err != nil {
		log.Fatal(err)
	}

	// Create a request to the Kubernetes API for forwarding ports
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", service.Namespace, podName)
	serverURL := url.URL{
		Scheme: "https",
		Path:   path,
		Host:   strings.TrimPrefix(config.Host, "https://"),
	}
	req := clientset.CoreV1().RESTClient().
		Post().
		RequestURI(serverURL.String())

	// Start port forwarding
	stopChan := make(chan struct{}, 1)
	readyChan := make(chan struct{})
	ports := []string{fmt.Sprintf("%d:%d", service.LocalPort, service.RemotePort)}

	forwarder, err := portforward.New(
		spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, "POST", req.URL()),
		ports,
		stopChan,
		readyChan,
		ioutil.Discard,
		ioutil.Discard,
	)
	if err != nil {
		log.Fatalf("Error setting up port forwarding: %s", err)
	}

	// Run forwarding in a goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Printf("Forwarding %s:%d to %s:%d", "localhost", service.LocalPort, service.Name, service.RemotePort)
		if err := forwarder.ForwardPorts(); err != nil {
			log.Fatalf("Error in port forwarding: %s", err)
		}
	}()

	// Wait for forwarding to be ready
	<-readyChan
}

func main() {

	if os.Getenv("APP_MODE") == "debug" {
		fmt.Println("You have 20 seconds to attach the Debugger...")
		time.Sleep(20 * time.Second) // waits for 20 seconds
		fmt.Println("Continuing...")
	}

	// Read the config
	services, err := readConfig("services.yaml")
	if err != nil {
		log.Fatalf("Error reading YAML file: %s", err)
	}

	var wg sync.WaitGroup
	// Iterate over services and set up port forwarding
	for _, profile := range services.Profiles {
		for _, service := range profile.Services {
			wg.Add(1)
			go setupPortForward(service, &wg)
		}
	}

	wg.Wait()
}
