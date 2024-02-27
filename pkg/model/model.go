package model

type Connection struct {
	ServiceName       string `yaml:"ServiceName,omitempty"`
	PodName           string `yaml:"PodName,omitempty"`
	RemoteServicePort int    `yaml:"RemoteServicePort,omitempty"`
	RemotePodPort     int    `yaml:"RemotePodPort,omitempty"` // Using a pointer to allow for empty values
	Namespace         string `yaml:"Namespace"`
	LocalPort         int    `yaml:"LocalPort"`
}

type Context struct {
	Name        string       `yaml:"Name"`
	Connections []Connection `yaml:"Connections"`
}

// Define a struct to hold the entire collection of contexts.
type Contexts struct {
	Contexts []Context `yaml:"Contexts"`
}

type PortForwardStatus struct {
	ServiceName string
	Err         error
}
