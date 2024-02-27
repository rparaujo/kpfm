package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v2"
	"k8s.io/client-go/util/homedir"

	"github.com/rparaujo/kpfm/pkg/kube"
	"github.com/rparaujo/kpfm/pkg/model"
)

func readConfig(filename string) (*model.Contexts, error) {
	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	c := &model.Contexts{}
	err = yaml.Unmarshal(buf, c)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func main() {

	err := createConfigFile()
	if err != nil {
		log.Fatalf("Error creating config file: %s", err)
		return // Exit early
	}

	currentContext, err := kube.GetCurrentContext()
	if err != nil {
		log.Fatalf("Error getting current context: %s", err)
	}

	config, err := readConfig(fmt.Sprintf("%s/.config/kpfm/config.yaml", homedir.HomeDir()))
	if err != nil {
		log.Fatalf("Error reading YAML file: %s", err)
	}

	// Initialize synchronization primitives
	var wg sync.WaitGroup
	statusCh := make(chan model.PortForwardStatus)
	notifyChan := make(chan string)
	stopChans := make(map[string]chan struct{}) // Keep track of stop channels for each port forward

	checkInterval := 10 * time.Second // Adjusted to check every 10 seconds
	go kube.WatchContextChanges(notifyChan, checkInterval)

	// Start initial port forwarding
	startPF(&wg, statusCh, currentContext, config, stopChans)

	for {
		select {
		case newContext := <-notifyChan:
			fmt.Printf("Kubecontext changed to: %s\n", newContext)
			// Stop all existing port forwards
			for _, stopChan := range stopChans {
				close(stopChan)
			}
			stopChans = make(map[string]chan struct{}) // Reset stop channels map
			wg.Wait()                                  // Wait for all port forwards to stop

			// Start new port forwards
			startPF(&wg, statusCh, newContext, config, stopChans)

		case status, ok := <-statusCh:
			if !ok {
				log.Println("Port-forward status channel closed")
				break
			}
			if status.Err != nil {
				log.Printf("Port-forward for %s stopped: %v", status.ServiceName, status.Err)
				// Restart port-forwarding for the service
				// This assumes you have a function to find the connection details by service name
				connection, found := findConnectionByServiceName(config, status.ServiceName, currentContext)
				if found {
					go kube.SetupPortForward(connection, &wg, statusCh, stopChans[status.ServiceName])
				}
			}
		}
	}
}

func startPF(wg *sync.WaitGroup, statusCh chan model.PortForwardStatus, context string, contexts *model.Contexts, stopChans map[string]chan struct{}) {
	for _, ctx := range contexts.Contexts {
		if ctx.Name == context {
			for _, connection := range ctx.Connections {
				stopChan := make(chan struct{})
				stopChans[connection.ServiceName] = stopChan // Track stop channel for each service
				wg.Add(1)
				go kube.SetupPortForward(connection, wg, statusCh, stopChan)
			}
		}
	}
}

// findConnectionByServiceName searches for a connection by its service name within the specified context.
// It returns the found connection and a boolean indicating whether the connection was found.
func findConnectionByServiceName(contexts *model.Contexts, serviceName, contextName string) (model.Connection, bool) {
	for _, ctx := range contexts.Contexts {
		if ctx.Name == contextName {
			for _, conn := range ctx.Connections {
				if conn.ServiceName == serviceName {
					return conn, true
				}
			}
		}
	}
	return model.Connection{}, false
}

func createConfigFile() error {
	dirPath := fmt.Sprintf("%s/.config/kpfm", homedir.HomeDir())
	filePath := dirPath + "/config.yaml"

	// Step 1: Create the directory if it doesn't exist
	err := os.MkdirAll(dirPath, 0755) // Permissions are set to rwxr-xr-x
	if err != nil {
		log.Fatalf("Failed to create directory: %s", err)
		return err
	}

	// Step 2: Create the file only if it does not exist
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if os.IsExist(err) {
			log.Printf("File already exists: %s", filePath)
		} else {
			log.Fatalf("Failed to create file: %s", err)
			return err
		}
		return nil
	}
	defer file.Close()
	return nil
}
