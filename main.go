package main

import (
	"context"
	"errors"
	"fmt"
	"io"
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

type PortForwardStatus struct {
	ServiceName string
	Err         error
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

func setupPortForward(service Service, wg *sync.WaitGroup, statusCh chan<- PortForwardStatus) {
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

	logWriter := io.MultiWriter(os.Stdout)

	forwarder, err := portforward.New(
		spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, "POST", req.URL()),
		ports,
		stopChan,
		readyChan,
		logWriter, // Use logWriter for standard output
		logWriter, // Use logWriter for standard error
	)
	if err != nil {
		log.Fatalf("Error setting up port forwarding: %s", err)
	}

	// Run forwarding in a goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := forwarder.ForwardPorts()
		// Send status to the channel when port-forwarding stops
		statusCh <- PortForwardStatus{ServiceName: service.Name, Err: err}
	}()

	// Wait for forwarding to be ready
	<-readyChan
}

// Helper function to find a service by name
func findService(config *Config, serviceName string) Service {
	for _, profile := range config.Profiles {
		for _, service := range profile.Services {
			if service.Name == serviceName {
				return service
			}
		}
	}
	return Service{} // Return an empty service if not found
}

func main() {

	if os.Getenv("APP_MODE") == "debug" {
		fmt.Println("You have 20 seconds to attach the Debugger...")
		time.Sleep(20 * time.Second) // waits for 20 seconds
		fmt.Println("Continuing...")
	}

	// Read the config
	services, err := readConfig(fmt.Sprintf("%s/.config/.kpf", homedir.HomeDir()))
	if err != nil {
		log.Fatalf("Error reading YAML file: %s", err)
	}

	statusCh := make(chan PortForwardStatus)
	var wg sync.WaitGroup
	// Iterate over services and set up port forwarding
	for _, profile := range services.Profiles {
		for _, service := range profile.Services {
			wg.Add(1)
			go setupPortForward(service, &wg, statusCh)
		}
	}

	// Monitor port-forwarding status
	go func() {
		for status := range statusCh {
			if status.Err != nil {
				log.Printf("Port-forward for %s stopped: %v", status.ServiceName, status.Err)
				// Restart port-forwarding for the service
				wg.Add(1)
				go setupPortForward(findService(services, status.ServiceName), &wg, statusCh)
			}
		}
	}()

	wg.Wait()
}
