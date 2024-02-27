package kube

import (
	"fmt"
	"os"
	"time"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// getCurrentContext reads the current kubecontext from the kubeconfig file.
func GetCurrentContext() (string, error) {
	// Find the kubeconfig file.
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = home + "/.kube/config"
		} else {
			return "", fmt.Errorf("cannot find kubeconfig file")
		}
	}

	// Load the kubeconfig file to get the config.
	config, err := clientcmd.LoadFromFile(kubeconfig)
	if err != nil {
		return "", fmt.Errorf("cannot load kubeconfig file: %v", err)
	}

	// Return the current context name.
	return config.CurrentContext, nil
}

// watchContextChanges periodically checks for changes in the current kubecontext and notifies via a channel.
func WatchContextChanges(notifyChan chan<- string, checkInterval time.Duration) {
	var lastContext string

	for range time.Tick(checkInterval) {
		currentContext, err := GetCurrentContext()
		if err != nil {
			fmt.Printf("Error getting current context: %v\n", err)
			continue
		}

		if currentContext != lastContext && lastContext != "" {
			notifyChan <- currentContext
		}
		lastContext = currentContext
	}
}
