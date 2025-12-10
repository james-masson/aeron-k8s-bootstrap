// aeron-k8s-bootstrap - Kubernetes startup shim for Aeron media drivers
// Copyright (C) 2025 JMIPS Ltd.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// PodInfo holds information about a media driver pod
type PodInfo struct {
	Name         string
	IP           string
	CreationTime time.Time
}

type NetworkStatus struct {
	Name      string   `json:"name"`
	Interface string   `json:"interface"`
	IPs       []string `json:"ips,omitempty"`
	MAC       string   `json:"mac,omitempty"`
	Default   bool     `json:"default,omitempty"`
	DNS       struct{} `json:"dns"`
}

const (
	networkStatusAnnotation = "k8s.v1.cni.cncf.io/network-status"
	defaultSecondaryInterfaceName  = "net1"
	defaultSecondaryInterfaceNetworkName = "aeron-network"
)

// getInClusterConfig creates a Kubernetes client using in-cluster configuration
func getInClusterConfig() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %v", err)
	}

	return clientset, nil
}

// getCurrentNamespace reads the current namespace from the service account token
func getCurrentNamespace() (string, error) {
	namespaceFile := "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

	data, err := os.ReadFile(namespaceFile)
	if err != nil {
		log.Printf("Warning: Could not read namespace file, using 'default': %v", err)
		return "default", nil
	}

	return string(data), nil
}

// getMediaDriverPods finds all media driver pods with IP addresses, sorted by age, with optional limit
func getMediaDriverPods(clientset kubernetes.Interface, namespace, labelSelector string, maxPods int) ([]PodInfo, error) {
	log.Printf("Searching for media driver pods in namespace: %s with label selector: %s", namespace, labelSelector)

	// List pods with the media driver label
	listOptions := metav1.ListOptions{
		LabelSelector: labelSelector,
	}

	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), listOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %v", err)
	}

	var runningPods []PodInfo

	for _, pod := range pods.Items {

		// get secondary interface IP if available
		// fallback to primary PodIP if secondary is not found
		ip, err := getIP(pod)
		if err != nil {
			return nil, fmt.Errorf("failed to get IP for pod %s: %v", pod.Name, err)
		} 

		// Only filter on IP address - include all pods with IPs regardless of status
		if pod.Status.PodIP != "" {
			podInfo := PodInfo{
				Name:         pod.Name,
				IP:           ip,
				CreationTime: pod.CreationTimestamp.Time,
			}
			runningPods = append(runningPods, podInfo)
			log.Printf("Found media driver pod: %s in phase %s created at %v",
				pod.Name, pod.Status.Phase, pod.CreationTimestamp.Time)
		}
	}

	if len(runningPods) == 0 {
		log.Println("No media driver pods with IP addresses found")
		return nil, nil
	}

	// Sort by creation timestamp from oldest to newest
	sort.Slice(runningPods, func(i, j int) bool {
		return runningPods[i].CreationTime.Before(runningPods[j].CreationTime)
	})

	// Apply max pods limit if specified (0 means unlimited)
	if maxPods > 0 && len(runningPods) > maxPods {
		runningPods = runningPods[:maxPods]
		log.Printf("Limited to %d oldest pods (out of %d total)", maxPods, len(runningPods))
	}

	log.Printf("Found %d media driver pods with IP addresses", len(runningPods))
	for _, pod := range runningPods {
		log.Printf("Pod: %s (%s)", pod.Name, pod.IP)
	}

	return runningPods, nil
}

// unmarshalNetworkStatus parses the network status annotation JSON into a slice of NetworkStatus
func unmarshalNetworkStatus(annotation string) ([]NetworkStatus, error) {
	var networks []NetworkStatus
	err := json.Unmarshal([]byte(annotation), &networks)
	if err != nil {
		if err.Error() == "unexpected end of JSON input" {
			// Empty annotation, return empty slice
			return networks, nil
		}
		return nil, fmt.Errorf("error unmarshaling network status: %v", err)
	}
	return networks, nil
}

// getIP retrieves the IP address for the secondary interface from the pod's network status annotation
// it falls back to the primary PodIP if no secondary interface (network status annotation) is found
func getIP(pod v1.Pod) (string, error) {

	var networks []NetworkStatus
	networks, err := unmarshalNetworkStatus(pod.Annotations[networkStatusAnnotation])
	if err != nil {
		log.Printf("Error parsing network status for pod %s: %v", pod.Name, err)
		return "", err
	}

	if len(networks) == 0 {
		log.Printf("No network status annotation found for pod %s. Using status.PodIP", pod.Name)
		return pod.Status.PodIP, nil
	}

	secondaryInterfaceNetworkName, networkNameIsSet := os.LookupEnv("AERON_MD_SECONDARY_INTERFACE_NETWORK_NAME")
	secondaryInterfaceName, interfaceNameIsSet := os.LookupEnv("AERON_MD_SECONDARY_INTERFACE_NAME")

	for _, network := range networks {
		if networkNameIsSet && network.Name == secondaryInterfaceNetworkName {
			log.Printf("AERON_MD_SECONDARY_INTERFACE_NETWORK_NAME is set, found network %s for pod %s", secondaryInterfaceNetworkName, pod.Name)
			return network.IPs[0], nil
		} else if interfaceNameIsSet && network.Interface == secondaryInterfaceName {
			log.Printf("AERON_MD_SECONDARY_INTERFACE_NAME is set, found interface %s for pod %s", secondaryInterfaceName, pod.Name)
			return network.IPs[0], nil
		} else if network.Interface == defaultSecondaryInterfaceName {
			log.Printf("No secondary interface or network env var is set, found default secondary interface %s for pod %s", defaultSecondaryInterfaceName, pod.Name)
			return network.IPs[0], nil
		} 
	}

	log.Printf("network-status annotation was found, but no network matched default interface name %s for pod %s. Falling back to using its primary interface (status.PodIP)", defaultSecondaryInterfaceName, pod.Name)
	return pod.Status.PodIP, nil
}

// getLabelSelector returns the label selector from environment variable or default
func getLabelSelector() string {
	if selector := os.Getenv("AERON_MD_LABEL_SELECTOR"); selector != "" {
		return selector
	}
	return "aeron.io/media-driver=true"
}

// getBootstrapPath returns the bootstrap file path from environment variable or default
func getBootstrapPath() string {
	if path := os.Getenv("AERON_MD_BOOTSTRAP_PATH"); path != "" {
		return path
	}
	return "/etc/aeron/bootstrap.properties"
}

// getMaxPods returns the maximum number of pods to include from environment variable or default
func getMaxPods() int {
	if maxStr := os.Getenv("AERON_MD_MAX_BOOTSTRAP_PODS"); maxStr != "" {
		if max, err := strconv.Atoi(maxStr); err == nil && max >= 0 {
			return max
		}
		log.Printf("Invalid AERON_MD_MAX_BOOTSTRAP_PODS value '%s', using default 0 (unlimited)", maxStr)
	}
	return 0
}

// getNamespace returns the namespace from environment variable or discovers it
func getNamespace() (string, error) {
	if namespace := os.Getenv("AERON_MD_NAMESPACE"); namespace != "" {
		return namespace, nil
	}
	return getCurrentNamespace()
}

// getHostnameSuffix returns the hostname suffix from environment variable or default
func getHostnameSuffix() string {
	if suffix := os.Getenv("AERON_MD_HOSTNAME_SUFFIX"); suffix != "" {
		return suffix
	}
	return ".aeron"
}

// getDiscoveryPort returns the discovery port from environment variable or default
func getDiscoveryPort() int {
	if portStr := os.Getenv("AERON_MD_DISCOVERY_PORT"); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil && port > 0 && port <= 65535 {
			return port
		}
		log.Printf("Invalid AERON_MD_DISCOVERY_PORT value '%s', using default 8050", portStr)
	}
	return 8050
}

// getCurrentHostname returns the current pod's hostname
func getCurrentHostname() string {
	if hostname := os.Getenv("HOSTNAME"); hostname != "" {
		return hostname
	}
	if hostname, err := os.Hostname(); err == nil {
		return hostname
	}
	log.Println("Warning: Could not determine hostname, using 'localhost'")
	return "localhost"
}

// buildAeronHostname creates the full Aeron hostname with namespace and suffix
func buildAeronHostname(namespace string) string {
	baseHostname := getCurrentHostname()
	suffix := getHostnameSuffix()
	return fmt.Sprintf("%s.%s%s", baseHostname, namespace, suffix)
}

// createBootstrapProperties creates the bootstrap properties file with all neighbor IPs
func createBootstrapProperties(neighborIPs []string, discoveryPort int, fullHostname string) error {
	bootstrapPath := getBootstrapPath()
	dir := filepath.Dir(bootstrapPath)
	shortHostname := getCurrentHostname()
	return createBootstrapPropertiesAtPath(dir, bootstrapPath, neighborIPs, discoveryPort, fullHostname, shortHostname)
}

// createBootstrapPropertiesInDir creates the bootstrap properties file in a specified directory (for testing)
func createBootstrapPropertiesInDir(dir string, neighborIPs []string, discoveryPort int, fullHostname, shortHostname string) error {
	filePath := filepath.Join(dir, "bootstrap.properties")
	return createBootstrapPropertiesAtPath(dir, filePath, neighborIPs, discoveryPort, fullHostname, shortHostname)
}

// createBootstrapPropertiesAtPath creates the bootstrap properties file at a specified path
func createBootstrapPropertiesAtPath(dir, filePath string, neighborIPs []string, discoveryPort int, fullHostname, shortHostname string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %v", dir, err)
	}

	// Create comma-separated list of IP:port pairs
	var neighbors []string
	for _, ip := range neighborIPs {
		neighbors = append(neighbors, fmt.Sprintf("%s:%d", ip, discoveryPort))
	}

	// Create the properties content with resolver configuration
	var contentLines []string
	if len(neighbors) > 0 {
		contentLines = append(contentLines, fmt.Sprintf("aeron.driver.resolver.bootstrap.neighbor=%s", strings.Join(neighbors, ",")))
	}
	contentLines = append(contentLines, "aeron.name.resolver.supplier=driver")

	contentLines = append(contentLines, fmt.Sprintf("aeron.driver.resolver.name=%s", fullHostname))
	contentLines = append(contentLines, fmt.Sprintf("aeron.driver.resolver.interface=%s:%d", shortHostname, discoveryPort))
	content := strings.Join(contentLines, "\n") + "\n"

	// Write the file
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write bootstrap properties file: %v", err)
	}

	if len(neighbors) > 0 {
		log.Printf("Created %s with bootstrap neighbors: %s, media-driver name: %s, interface: %s:%d", filePath, strings.Join(neighbors, ","), fullHostname, shortHostname, discoveryPort)
	} else {
		log.Printf("Created %s with media-driver name: %s, interface: %s:%d (no neighbors found)", filePath, fullHostname, shortHostname, discoveryPort)
	}

	return nil
}

func main() {
	log.Println("Starting Aeron bootstrap neighbor discovery...")

	// Create Kubernetes client
	clientset, err := getInClusterConfig()
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	// Get namespace (from env var or auto-discover)
	namespace, err := getNamespace()
	if err != nil {
		log.Fatalf("Failed to determine namespace: %v", err)
	}

	// Get configuration
	labelSelector := getLabelSelector()
	maxPods := getMaxPods()

	// Find all media driver pods
	pods, err := getMediaDriverPods(clientset, namespace, labelSelector, maxPods)
	if err != nil {
		log.Fatalf("Error finding media driver pods: %v", err)
	}

	if len(pods) == 0 {
		log.Println("Error: No suitable media driver pods found. Exiting without creating bootstrap file.")
		os.Exit(1)
	}

	// Extract IPs from pods (already sorted oldest to newest)
	var neighborIPs []string
	for _, pod := range pods {
		neighborIPs = append(neighborIPs, pod.IP)
	}

	// Get configuration
	discoveryPort := getDiscoveryPort()
	aeronHostname := buildAeronHostname(namespace)

	// Create the bootstrap properties file
	if err := createBootstrapProperties(neighborIPs, discoveryPort, aeronHostname); err != nil {
		log.Fatalf("Error creating bootstrap properties file: %v", err)
	}

	log.Println("Bootstrap neighbor discovery completed successfully")
}
