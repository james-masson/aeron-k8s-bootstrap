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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetMediaDriverPodsWithSecondaryInterface(t *testing.T) {
	tests := []struct {
		name          string
		pods          []corev1.Pod
		expected      []PodInfo
		networkName   string
		interfaceName string
	}{
		{
			name: "pod with secondary interface",
			pods: []corev1.Pod{
				createTestPodWithSecondaryInterface("aeron-0", "10.0.0.1", "10.0.0.2", "Running", "aeron-network", "net1", time.Now().Add(-2*time.Minute)),
			},
			expected: []PodInfo{
				{Name: "aeron-older", IP: "10.0.0.2", CreationTime: time.Now().Add(-10 * time.Minute)},
			},
		},
		{
			name: "pod without a secondary interface",
			pods: []corev1.Pod{
				createTestPod("aeron-0", "10.0.0.1", "Running", time.Now().Add(-2*time.Minute)),
			},
			expected: []PodInfo{
				{Name: "aeron-older", IP: "10.0.0.1", CreationTime: time.Now().Add(-10 * time.Minute)},
			},
		},
		{
			name: "pod with a secondary interface and network env var",
			pods: []corev1.Pod{
				createTestPodWithSecondaryInterface("aeron-0", "10.0.0.1", "10.0.0.2", "Running", "custom-network", "", time.Now().Add(-2*time.Minute)),
			},
			expected: []PodInfo{
				{Name: "aeron-older", IP: "10.0.0.2", CreationTime: time.Now().Add(-10 * time.Minute)},
			},
			networkName: "custom-network",
		},
		{
			name: "pod with a secondary interface and interface name env var",
			pods: []corev1.Pod{
				createTestPodWithSecondaryInterface("aeron-0", "10.0.0.1", "10.0.0.2", "Running", "", "custom1", time.Now().Add(-2*time.Minute)),
			},
			expected: []PodInfo{
				{Name: "aeron-older", IP: "10.0.0.2", CreationTime: time.Now().Add(-10 * time.Minute)},
			},
			interfaceName: "custom1",
		},
		{
			name: "pod with a secondary interface and interface and network env vars",
			pods: []corev1.Pod{
				createTestPodWithSecondaryInterface("aeron-0", "10.0.0.1", "10.0.0.2", "Running", "custom-network", "custom1", time.Now().Add(-2*time.Minute)),
			},
			expected: []PodInfo{
				{Name: "aeron-older", IP: "10.0.0.2", CreationTime: time.Now().Add(-10 * time.Minute)},
			},
			networkName:   "custom-network",
			interfaceName: "custom1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()

			// Add pods to the fake client
			for _, pod := range tt.pods {
				_, err := clientset.CoreV1().Pods("test-namespace").Create(context.TODO(), &pod, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Failed to create test pod: %v", err)
				}
			}

			os.Unsetenv("AERON_MD_SECONDARY_INTERFACE_NETWORK_NAME")
			os.Unsetenv("AERON_MD_SECONDARY_INTERFACE_NAME")
			if tt.networkName != "" {
				// Set the environment variable for secondary interface network name
				t.Setenv("AERON_MD_SECONDARY_INTERFACE_NETWORK_NAME", tt.networkName)
			}
			if tt.interfaceName != "" {
				// Set the environment variable for secondary interface name
				t.Setenv("AERON_MD_SECONDARY_INTERFACE_NAME", tt.interfaceName)
			}
			result, err := getMediaDriverPods(clientset, "test-namespace", "aeron.io/media-driver=true", 0)
			if err != nil {
				t.Fatalf("getMediaDriverPods() error = %v", err)
			}

			if len(result) != len(tt.expected) {
				t.Errorf("getMediaDriverPods() returned %d pods, expected %d", len(result), len(tt.expected))
				return
			}

			if result[0].IP != tt.expected[0].IP {
				t.Errorf("Pod IP = %s, expected %s", result[0].IP, tt.expected[0].IP)
			}
		})
	}
}

func TestGetMediaDriverPods(t *testing.T) {
	tests := []struct {
		name     string
		pods     []corev1.Pod
		expected []PodInfo
	}{
		{
			name:     "no pods found",
			pods:     []corev1.Pod{},
			expected: nil,
		},
		{
			name: "single pod with IP",
			pods: []corev1.Pod{
				createTestPod("aeron-1", "10.0.0.1", "Running", time.Now().Add(-5*time.Minute)),
			},
			expected: []PodInfo{
				{Name: "aeron-1", IP: "10.0.0.1", CreationTime: time.Now().Add(-5 * time.Minute)},
			},
		},
		{
			name: "multiple pods sorted by creation time",
			pods: []corev1.Pod{
				createTestPod("aeron-newer", "10.0.0.2", "Running", time.Now().Add(-2*time.Minute)),
				createTestPod("aeron-older", "10.0.0.1", "Running", time.Now().Add(-10*time.Minute)),
				createTestPod("aeron-middle", "10.0.0.3", "Running", time.Now().Add(-5*time.Minute)),
			},
			expected: []PodInfo{
				{Name: "aeron-older", IP: "10.0.0.1", CreationTime: time.Now().Add(-10 * time.Minute)},
				{Name: "aeron-middle", IP: "10.0.0.3", CreationTime: time.Now().Add(-5 * time.Minute)},
				{Name: "aeron-newer", IP: "10.0.0.2", CreationTime: time.Now().Add(-2 * time.Minute)},
			},
		},
		{
			name: "pods without IP addresses filtered out",
			pods: []corev1.Pod{
				createTestPod("aeron-with-ip", "10.0.0.1", "Running", time.Now().Add(-5*time.Minute)),
				createTestPodWithoutIP("aeron-without-ip", "Pending", time.Now().Add(-3*time.Minute)),
			},
			expected: []PodInfo{
				{Name: "aeron-with-ip", IP: "10.0.0.1", CreationTime: time.Now().Add(-5 * time.Minute)},
			},
		},
		{
			name: "pods in different phases but with IP addresses included",
			pods: []corev1.Pod{
				createTestPod("aeron-running", "10.0.0.1", "Running", time.Now().Add(-5*time.Minute)),
				createTestPod("aeron-terminating", "10.0.0.2", "Running", time.Now().Add(-3*time.Minute)),
			},
			expected: []PodInfo{
				{Name: "aeron-running", IP: "10.0.0.1", CreationTime: time.Now().Add(-5 * time.Minute)},
				{Name: "aeron-terminating", IP: "10.0.0.2", CreationTime: time.Now().Add(-3 * time.Minute)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()

			// Add pods to the fake client
			for _, pod := range tt.pods {
				_, err := clientset.CoreV1().Pods("test-namespace").Create(context.TODO(), &pod, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Failed to create test pod: %v", err)
				}
			}

			result, err := getMediaDriverPods(clientset, "test-namespace", "aeron.io/media-driver=true", 0)
			if err != nil {
				t.Fatalf("getMediaDriverPods() error = %v", err)
			}

			if len(result) != len(tt.expected) {
				t.Errorf("getMediaDriverPods() returned %d pods, expected %d", len(result), len(tt.expected))
				return
			}

			for i, pod := range result {
				if pod.Name != tt.expected[i].Name {
					t.Errorf("Pod %d name = %s, expected %s", i, pod.Name, tt.expected[i].Name)
				}
				if pod.IP != tt.expected[i].IP {
					t.Errorf("Pod %d IP = %s, expected %s", i, pod.IP, tt.expected[i].IP)
				}
				// Allow some tolerance for time comparison due to test execution time
				if pod.CreationTime.Sub(tt.expected[i].CreationTime).Abs() > time.Second {
					t.Errorf("Pod %d creation time = %v, expected %v", i, pod.CreationTime, tt.expected[i].CreationTime)
				}
			}
		})
	}
}

func TestCreateBootstrapProperties(t *testing.T) {
	tests := []struct {
		name          string
		neighborIPs   []string
		discoveryPort int
		hostname      string
		expectedLines []string
	}{
		{
			name:          "no neighbors",
			neighborIPs:   []string{},
			discoveryPort: 8050,
			hostname:      "test-host",
			expectedLines: []string{
				"aeron.name.resolver.supplier=driver",
				"aeron.driver.resolver.name=test-host",
				"aeron.driver.resolver.interface=test-host:8050",
			},
		},
		{
			name:          "single neighbor",
			neighborIPs:   []string{"10.0.0.1"},
			discoveryPort: 8050,
			hostname:      "test-host",
			expectedLines: []string{
				"aeron.driver.resolver.bootstrap.neighbor=10.0.0.1:8050",
				"aeron.name.resolver.supplier=driver",
				"aeron.driver.resolver.name=test-host",
				"aeron.driver.resolver.interface=test-host:8050",
			},
		},
		{
			name:          "multiple neighbors",
			neighborIPs:   []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"},
			discoveryPort: 9090,
			hostname:      "aeron-pod-123",
			expectedLines: []string{
				"aeron.driver.resolver.bootstrap.neighbor=10.0.0.1:9090,10.0.0.2:9090,10.0.0.3:9090",
				"aeron.name.resolver.supplier=driver",
				"aeron.driver.resolver.name=aeron-pod-123",
				"aeron.driver.resolver.interface=aeron-pod-123:9090",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// Set the hostname for the test
			t.Setenv("HOSTNAME", tt.hostname)

			// Create temporary directory for test
			tempDir, err := os.MkdirTemp("", "aeron-test-")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tempDir)

			// Use the testable function with custom directory
			aeronDir := filepath.Join(tempDir, "aeron")

			err = createBootstrapPropertiesInDir(aeronDir, tt.neighborIPs, tt.discoveryPort, tt.hostname, tt.hostname)
			if err != nil {
				t.Fatalf("createBootstrapProperties() error = %v", err)
			}

			// Read the created file
			filePath := filepath.Join(aeronDir, "bootstrap.properties")
			content, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatalf("Failed to read bootstrap properties file: %v", err)
			}

			lines := strings.Split(strings.TrimSpace(string(content)), "\n")

			if len(lines) != len(tt.expectedLines) {
				t.Errorf("Expected %d lines, got %d. Content:\n%s", len(tt.expectedLines), len(lines), string(content))
				return
			}

			for i, line := range lines {
				if line != tt.expectedLines[i] {
					t.Errorf("Line %d: got \"%s\", expected \"%s\"", i, line, tt.expectedLines[i])
				}
			}
		})
	}
}

func TestCreateBootstrapPropertiesWithNamespaceHostname(t *testing.T) {
	tests := []struct {
		name          string
		neighborIPs   []string
		discoveryPort int
		namespace     string
		podHostname   string
		suffix        string
		expectedLines []string
	}{
		{
			name:          "standard case with namespace",
			neighborIPs:   []string{"10.0.0.1", "10.0.0.2"},
			discoveryPort: 8050,
			namespace:     "uat",
			podHostname:   "server1",
			suffix:        ".aeron",
			expectedLines: []string{
				"aeron.driver.resolver.bootstrap.neighbor=10.0.0.1:8050,10.0.0.2:8050",
				"aeron.name.resolver.supplier=driver",
				"aeron.driver.resolver.name=server1.uat.aeron",
				"aeron.driver.resolver.interface=server1:8050",
			},
		},
		{
			name:          "production with custom suffix",
			neighborIPs:   []string{"192.168.1.10"},
			discoveryPort: 9090,
			namespace:     "production",
			podHostname:   "aeron-md-1",
			suffix:        ".cluster",
			expectedLines: []string{
				"aeron.driver.resolver.bootstrap.neighbor=192.168.1.10:9090",
				"aeron.name.resolver.supplier=driver",
				"aeron.driver.resolver.name=aeron-md-1.production.cluster",
				"aeron.driver.resolver.interface=aeron-md-1:9090",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// Set test env values
			t.Setenv("HOSTNAME", tt.podHostname)
			t.Setenv("AERON_MD_HOSTNAME_SUFFIX", tt.suffix)

			// Create temporary directory for test
			tempDir, err := os.MkdirTemp("", "aeron-hostname-test-")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tempDir)

			// Use the testable function with custom directory
			aeronDir := filepath.Join(tempDir, "aeron")

			// Build the full hostname with namespace
			fullHostname := buildAeronHostname(tt.namespace)

			err = createBootstrapPropertiesInDir(aeronDir, tt.neighborIPs, tt.discoveryPort, fullHostname, tt.podHostname)
			if err != nil {
				t.Fatalf("createBootstrapProperties() error = %v", err)
			}

			// Read the created file
			filePath := filepath.Join(aeronDir, "bootstrap.properties")
			content, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatalf("Failed to read bootstrap properties file: %v", err)
			}

			lines := strings.Split(strings.TrimSpace(string(content)), "\n")

			if len(lines) != len(tt.expectedLines) {
				t.Errorf("Expected %d lines, got %d. Content:\n%s", len(tt.expectedLines), len(lines), string(content))
				return
			}

			for i, line := range lines {
				if line != tt.expectedLines[i] {
					t.Errorf("Line %d: got \"%s\", expected \"%s\"", i, line, tt.expectedLines[i])
				}
			}
		})
	}
}

func TestGetDiscoveryPort(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected int
	}{
		{
			name:     "default port when env not set",
			envValue: "",
			expected: 8050,
		},
		{
			name:     "valid env port",
			envValue: "9090",
			expected: 9090,
		},
		{
			name:     "invalid env port - non-numeric",
			envValue: "invalid",
			expected: 8050,
		},
		{
			name:     "invalid env port - out of range",
			envValue: "99999",
			expected: 8050,
		},
		{
			name:     "invalid env port - zero",
			envValue: "0",
			expected: 8050,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test env value
			if tt.envValue != "" {
				t.Setenv("AERON_MD_DISCOVERY_PORT", tt.envValue)
			} else {
				os.Unsetenv("AERON_MD_DISCOVERY_PORT")
			}

			result := getDiscoveryPort()
			if result != tt.expected {
				t.Errorf("getDiscoveryPort() = %d, expected %d", result, tt.expected)
			}
		})
	}
}

func TestGetLabelSelector(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected string
	}{
		{
			name:     "default label when env not set",
			envValue: "",
			expected: "aeron.io/media-driver=true",
		},
		{
			name:     "custom label from environment",
			envValue: "app=aeron,version=1.0",
			expected: "app=aeron,version=1.0",
		},
		{
			name:     "single custom label",
			envValue: "service=media-driver",
			expected: "service=media-driver",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test env value
			if tt.envValue != "" {
				t.Setenv("AERON_MD_LABEL_SELECTOR", tt.envValue)
			} else {
				os.Unsetenv("AERON_MD_LABEL_SELECTOR")
			}

			result := getLabelSelector()
			if result != tt.expected {
				t.Errorf("getLabelSelector() = %s, expected %s", result, tt.expected)
			}
		})
	}
}

func TestGetMediaDriverPodsWithCustomLabel(t *testing.T) {
	clientset := fake.NewSimpleClientset()

	// Create pods with different labels
	customPod := createTestPodWithLabel("custom-aeron", "10.0.0.1", "Running", time.Now().Add(-5*time.Minute), "app", "aeron-driver")
	defaultPod := createTestPodWithLabel("default-aeron", "10.0.0.2", "Running", time.Now().Add(-3*time.Minute), "aeron.io/media-driver", "true")

	// Add both pods to the fake client
	_, err := clientset.CoreV1().Pods("test-namespace").Create(context.TODO(), &customPod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create custom test pod: %v", err)
	}

	_, err = clientset.CoreV1().Pods("test-namespace").Create(context.TODO(), &defaultPod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create default test pod: %v", err)
	}

	// Test with custom label selector - should only find the custom pod
	result, err := getMediaDriverPods(clientset, "test-namespace", "app=aeron-driver", 0)
	if err != nil {
		t.Fatalf("getMediaDriverPods() error = %v", err)
	}

	if len(result) != 1 {
		t.Errorf("Expected 1 pod with custom label, got %d", len(result))
		return
	}

	if result[0].Name != "custom-aeron" {
		t.Errorf("Expected pod name \"custom-aeron\", got \"%s\"", result[0].Name)
	}

	// Test with default label selector - should only find the default pod
	result, err = getMediaDriverPods(clientset, "test-namespace", "aeron.io/media-driver=true", 0)
	if err != nil {
		t.Fatalf("getMediaDriverPods() error = %v", err)
	}

	if len(result) != 1 {
		t.Errorf("Expected 1 pod with default label, got %d", len(result))
		return
	}

	if result[0].Name != "default-aeron" {
		t.Errorf("Expected pod name \"default-aeron\", got \"%s\"", result[0].Name)
	}
}

func TestGetBootstrapPath(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected string
	}{
		{
			name:     "default path when env not set",
			envValue: "",
			expected: "/etc/aeron/bootstrap.properties",
		},
		{
			name:     "custom path from environment",
			envValue: "/custom/path/bootstrap.properties",
			expected: "/custom/path/bootstrap.properties",
		},
		{
			name:     "relative path",
			envValue: "./config/bootstrap.properties",
			expected: "./config/bootstrap.properties",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test env value
			if tt.envValue != "" {
				t.Setenv("AERON_MD_BOOTSTRAP_PATH", tt.envValue)
			} else {
				os.Unsetenv("AERON_MD_BOOTSTRAP_PATH")
			}

			result := getBootstrapPath()
			if result != tt.expected {
				t.Errorf("getBootstrapPath() = %s, expected %s", result, tt.expected)
			}
		})
	}
}

func TestGetMaxPods(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected int
	}{
		{
			name:     "default max pods when env not set",
			envValue: "",
			expected: 0,
		},
		{
			name:     "valid max pods",
			envValue: "5",
			expected: 5,
		},
		{
			name:     "zero max pods (unlimited)",
			envValue: "0",
			expected: 0,
		},
		{
			name:     "large max pods",
			envValue: "100",
			expected: 100,
		},
		{
			name:     "invalid env value - non-numeric",
			envValue: "invalid",
			expected: 0,
		},
		{
			name:     "invalid env value - negative",
			envValue: "-5",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// Set test env value
			if tt.envValue != "" {
				t.Setenv("AERON_MD_MAX_BOOTSTRAP_PODS", tt.envValue)
			} else {
				os.Unsetenv("AERON_MD_MAX_BOOTSTRAP_PODS")
			}

			result := getMaxPods()
			if result != tt.expected {
				t.Errorf("getMaxPods() = %d, expected %d", result, tt.expected)
			}
		})
	}
}

func TestGetMediaDriverPodsWithMaxLimit(t *testing.T) {
	clientset := fake.NewSimpleClientset()

	// Create 5 pods with different creation times
	pods := []corev1.Pod{
		createTestPod("aeron-1", "10.0.0.1", "Running", time.Now().Add(-10*time.Minute)),
		createTestPod("aeron-2", "10.0.0.2", "Running", time.Now().Add(-8*time.Minute)),
		createTestPod("aeron-3", "10.0.0.3", "Running", time.Now().Add(-6*time.Minute)),
		createTestPod("aeron-4", "10.0.0.4", "Running", time.Now().Add(-4*time.Minute)),
		createTestPod("aeron-5", "10.0.0.5", "Running", time.Now().Add(-2*time.Minute)),
	}

	// Add all pods to the fake client
	for _, pod := range pods {
		_, err := clientset.CoreV1().Pods("test-namespace").Create(context.TODO(), &pod, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create test pod: %v", err)
		}
	}

	// Test with no limit (0 = unlimited, should get all 5)
	result, err := getMediaDriverPods(clientset, "test-namespace", "aeron.io/media-driver=true", 0)
	if err != nil {
		t.Fatalf("getMediaDriverPods() error = %v", err)
	}
	if len(result) != 5 {
		t.Errorf("Expected 5 pods with unlimited (0), got %d", len(result))
	}

	// Test with limit of 3 (should get 3 oldest)
	result, err = getMediaDriverPods(clientset, "test-namespace", "aeron.io/media-driver=true", 3)
	if err != nil {
		t.Fatalf("getMediaDriverPods() error = %v", err)
	}
	if len(result) != 3 {
		t.Errorf("Expected 3 pods with limit, got %d", len(result))
	}

	// Verify we got the oldest pods
	expectedNames := []string{"aeron-1", "aeron-2", "aeron-3"}
	for i, pod := range result {
		if pod.Name != expectedNames[i] {
			t.Errorf("Pod %d: expected %s, got %s", i, expectedNames[i], pod.Name)
		}
	}

	// Test with limit larger than available pods
	result, err = getMediaDriverPods(clientset, "test-namespace", "aeron.io/media-driver=true", 10)
	if err != nil {
		t.Fatalf("getMediaDriverPods() error = %v", err)
	}
	if len(result) != 5 {
		t.Errorf("Expected 5 pods (all available) with large limit, got %d", len(result))
	}
}

func TestGetNamespace(t *testing.T) {
	tests := []struct {
		name         string
		envValue     string
		expectError  bool
		expectedName string
	}{
		{
			name:         "custom namespace from environment",
			envValue:     "custom-namespace",
			expectError:  false,
			expectedName: "custom-namespace",
		},
		{
			name:         "production namespace",
			envValue:     "production",
			expectError:  false,
			expectedName: "production",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// Set test env value
			if tt.envValue != "" {
				t.Setenv("AERON_MD_NAMESPACE", tt.envValue)
			} else {
				os.Unsetenv("AERON_MD_NAMESPACE")
			}

			result, err := getNamespace()

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if !tt.expectError && result != tt.expectedName {
				t.Errorf("getNamespace() = %s, expected %s", result, tt.expectedName)
			}
		})
	}
}

func TestGetHostnameSuffix(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected string
	}{
		{
			name:     "default suffix when env not set",
			envValue: "",
			expected: ".aeron",
		},
		{
			name:     "custom suffix from environment",
			envValue: ".custom",
			expected: ".custom",
		},
		{
			name:     "suffix without dot",
			envValue: "mysuffix",
			expected: "mysuffix",
		},
		{
			name:     "empty suffix",
			envValue: "",
			expected: ".aeron",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// Set test env value
			if tt.envValue != "" {
				t.Setenv("AERON_MD_HOSTNAME_SUFFIX", tt.envValue)
			} else {
				os.Unsetenv("AERON_MD_HOSTNAME_SUFFIX")
			}

			result := getHostnameSuffix()
			if result != tt.expected {
				t.Errorf("getHostnameSuffix() = %s, expected %s", result, tt.expected)
			}
		})
	}
}

func TestBuildAeronHostname(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		hostname  string
		suffix    string
		expected  string
	}{
		{
			name:      "standard case",
			namespace: "uat",
			hostname:  "server1",
			suffix:    ".aeron",
			expected:  "server1.uat.aeron",
		},
		{
			name:      "production namespace",
			namespace: "production",
			hostname:  "aeron-pod-123",
			suffix:    ".aeron",
			expected:  "aeron-pod-123.production.aeron",
		},
		{
			name:      "custom suffix",
			namespace: "test",
			hostname:  "myserver",
			suffix:    ".custom",
			expected:  "myserver.test.custom",
		},
		{
			name:      "custom suffix with dot",
			namespace: "dev",
			hostname:  "pod1",
			suffix:    ".cluster",
			expected:  "pod1.dev.cluster",
		},
		{
			name:      "suffix without dot",
			namespace: "dev",
			hostname:  "pod1",
			suffix:    "cluster",
			expected:  "pod1.devcluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// Set test env values
			t.Setenv("HOSTNAME", tt.hostname)
			if tt.suffix != "" {
				t.Setenv("AERON_MD_HOSTNAME_SUFFIX", tt.suffix)
			} else {
				os.Unsetenv("AERON_MD_HOSTNAME_SUFFIX")
			}

			result := buildAeronHostname(tt.namespace)
			if result != tt.expected {
				t.Errorf("buildAeronHostname(%s) = %s, expected %s", tt.namespace, result, tt.expected)
			}
		})
	}
}

func TestMainExitsWithErrorWhenNoPodsFound(t *testing.T) {
	// This test verifies that when no pods are found, the application exits with code 1
	// We can\"t easily test os.Exit(1) directly, but we can test the logic that leads to it

	clientset := fake.NewSimpleClientset()
	// Don\"t add any pods - this will simulate no pods found

	// Test that getMediaDriverPods returns empty result
	result, err := getMediaDriverPods(clientset, "test-namespace", "aeron.io/media-driver=true", 0)
	if err != nil {
		t.Fatalf("getMediaDriverPods() error = %v", err)
	}

	// Verify that no pods are returned (which should trigger exit code 1 in main)
	if len(result) != 0 {
		t.Errorf("Expected 0 pods when none exist, got %d", len(result))
	}
}

func TestMainExitsWithErrorWhenOnlyPodsWithoutIPFound(t *testing.T) {
	// This test verifies that pods without IP addresses are filtered out,
	// and if no pods with IPs remain, the application should exit with code 1

	clientset := fake.NewSimpleClientset()

	// Add pods without IP addresses
	podsWithoutIP := []corev1.Pod{
		createTestPodWithoutIP("aeron-pending-1", "Pending", time.Now().Add(-5*time.Minute)),
		createTestPodWithoutIP("aeron-pending-2", "Pending", time.Now().Add(-3*time.Minute)),
	}

	for _, pod := range podsWithoutIP {
		_, err := clientset.CoreV1().Pods("test-namespace").Create(context.TODO(), &pod, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create test pod: %v", err)
		}
	}

	// Test that getMediaDriverPods returns empty result (pods without IPs are filtered out)
	result, err := getMediaDriverPods(clientset, "test-namespace", "aeron.io/media-driver=true", 0)
	if err != nil {
		t.Fatalf("getMediaDriverPods() error = %v", err)
	}

	// Verify that no pods are returned (which should trigger exit code 1 in main)
	if len(result) != 0 {
		t.Errorf("Expected 0 pods when only pods without IPs exist, got %d", len(result))
	}
}

func TestMainExitsWithErrorWhenWrongLabelSelector(t *testing.T) {
	// This test verifies that when using a label selector that matches no pods,
	// the application should exit with code 1

	clientset := fake.NewSimpleClientset()

	// Add pods with the default label
	podsWithDefaultLabel := []corev1.Pod{
		createTestPod("aeron-1", "10.0.0.1", "Running", time.Now().Add(-5*time.Minute)),
		createTestPod("aeron-2", "10.0.0.2", "Running", time.Now().Add(-3*time.Minute)),
	}

	for _, pod := range podsWithDefaultLabel {
		_, err := clientset.CoreV1().Pods("test-namespace").Create(context.TODO(), &pod, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create test pod: %v", err)
		}
	}

	// Test with a label selector that won\"t match any pods
	result, err := getMediaDriverPods(clientset, "test-namespace", "app=nonexistent", 0)
	if err != nil {
		t.Fatalf("getMediaDriverPods() error = %v", err)
	}

	// Verify that no pods are returned (which should trigger exit code 1 in main)
	if len(result) != 0 {
		t.Errorf("Expected 0 pods with wrong label selector, got %d", len(result))
	}
}

func TestResolverInterfaceUsesShortHostname(t *testing.T) {
	tests := []struct {
		name          string
		neighborIPs   []string
		discoveryPort int
		fullHostname  string
		shortHostname string
		expectedLines []string
	}{
		{
			name:          "resolver.interface uses short hostname",
			neighborIPs:   []string{"10.0.0.1", "10.0.0.2"},
			discoveryPort: 8050,
			fullHostname:  "server1.uat.aeron",
			shortHostname: "server1",
			expectedLines: []string{
				"aeron.driver.resolver.bootstrap.neighbor=10.0.0.1:8050,10.0.0.2:8050",
				"aeron.name.resolver.supplier=driver",
				"aeron.driver.resolver.name=server1.uat.aeron",
				"aeron.driver.resolver.interface=server1:8050",
			},
		},
		{
			name:          "production with custom suffix - interface still short",
			neighborIPs:   []string{"192.168.1.10"},
			discoveryPort: 9090,
			fullHostname:  "aeron-md-1.production.cluster",
			shortHostname: "aeron-md-1",
			expectedLines: []string{
				"aeron.driver.resolver.bootstrap.neighbor=192.168.1.10:9090",
				"aeron.name.resolver.supplier=driver",
				"aeron.driver.resolver.name=aeron-md-1.production.cluster",
				"aeron.driver.resolver.interface=aeron-md-1:9090",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory for test
			tempDir, err := os.MkdirTemp("", "aeron-interface-test-")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tempDir)

			// Use the testable function with custom directory
			aeronDir := filepath.Join(tempDir, "aeron")

			err = createBootstrapPropertiesInDir(aeronDir, tt.neighborIPs, tt.discoveryPort, tt.fullHostname, tt.shortHostname)
			if err != nil {
				t.Fatalf("createBootstrapProperties() error = %v", err)
			}

			// Read the created file
			filePath := filepath.Join(aeronDir, "bootstrap.properties")
			content, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatalf("Failed to read bootstrap properties file: %v", err)
			}

			lines := strings.Split(strings.TrimSpace(string(content)), "\n")

			if len(lines) != len(tt.expectedLines) {
				t.Errorf("Expected %d lines, got %d. Content:\n%s", len(tt.expectedLines), len(lines), string(content))
				return
			}

			for i, line := range lines {
				if line != tt.expectedLines[i] {
					t.Errorf("Line %d: got \"%s\", expected \"%s\"", i, line, tt.expectedLines[i])
				}
			}
		})
	}
}

func TestGetCurrentHostname(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected string
	}{
		{
			name:     "hostname from environment",
			envValue: "test-pod-123",
			expected: "test-pod-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// Set test env value
			if tt.envValue != "" {
				t.Setenv("HOSTNAME", tt.envValue)
			} else {
				os.Unsetenv("HOSTNAME")
			}

			result := getCurrentHostname()
			if result != tt.expected {
				t.Errorf("getCurrentHostname() = %s, expected %s", result, tt.expected)
			}
		})
	}
}

func TestGetIP(t *testing.T) {
	tests := []struct {
		name     string
		pod      corev1.Pod
		expected string
	}{
		{
			name:     "pod with primary IP",
			pod:      createTestPod("pod-with-primary", "10.0.0.1", "Running", time.Now().Add(-5*time.Minute)),
			expected: "10.0.0.1",
		},
		{
			name: "pod with secondary interface",
			pod: createTestPodWithSecondaryInterface("pod-with-secondary", "10.0.0.1", "10.0.0.2", "Running", "custom-network", "net1", time.Now().Add(-5*time.Minute)),
			expected: "10.0.0.2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _ := getIP(tt.pod)
			if result != tt.expected {
				t.Errorf("getIP() = %s, expected %s", result, tt.expected)
			}
		})
	}
}

func TestGetCurrentPod(t *testing.T) {
	namespace := "test-namespace"
	pod := createTestPod("pod-with-primary", "10.0.0.1", "Running", time.Now().Add(-5*time.Minute))

	clientset := fake.NewSimpleClientset()
	_, err := clientset.CoreV1().Pods(namespace).Create(context.TODO(), &pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create test pod: %v", err)
	}
	t.Setenv("HOSTNAME", pod.Name)

	result := getCurrentPod(clientset, namespace)
	expected := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod-with-primary",
		},
		Status: corev1.PodStatus{
			PodIP: "10.0.0.1",
		},
	}

	if result.Name != expected.Name {
		t.Errorf("getCurrentPod() Name = %s, expected %s", result.Name, expected.Name)
	}
	if result.Status.PodIP != expected.Status.PodIP {
		t.Errorf("getCurrentPod() PodIP = %s, expected %s", result.Status.PodIP, expected.Status.PodIP)
	}
}

func TestParseNetworksAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotation  string
		expected    []string
		expectError bool
	}{
		{
			name:        "empty annotation",
			annotation:  "",
			expected:    nil,
			expectError: false,
		},
		{
			name:        "simple string",
			annotation:  "mynet",
			expected:    []string{"mynet"},
			expectError: false,
		},
		{
			name:        "comma-separated",
			annotation:  "mynet1,mynet2",
			expected:    []string{"mynet1", "mynet2"},
			expectError: false,
		},
		{
			name:        "comma-separated with spaces",
			annotation:  "mynet1, mynet2, mynet3",
			expected:    []string{"mynet1", "mynet2", "mynet3"},
			expectError: false,
		},
		{
			name:        "JSON array format",
			annotation:  `[{"name":"mynet1"},{"name":"mynet2"}]`,
			expected:    []string{"mynet1", "mynet2"},
			expectError: false,
		},
		{
			name:        "JSON array format with single network",
			annotation:  `[{"name":"custom-network"}]`,
			expected:    []string{"custom-network"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseNetworksAnnotation(tt.annotation)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d networks, got %d", len(tt.expected), len(result))
				return
			}

			for i, name := range result {
				if name != tt.expected[i] {
					t.Errorf("Network %d: expected %s, got %s", i, tt.expected[i], name)
				}
			}
		})
	}
}

func TestValidateMultusNetworkStatus(t *testing.T) {
	tests := []struct {
		name     string
		pod      corev1.Pod
		expected bool
	}{
		{
			name: "pod without networks annotation - valid",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "pod-no-multus",
					Annotations: map[string]string{},
				},
			},
			expected: true,
		},
		{
			name: "pod with networks annotation but no network-status - invalid",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-missing-status",
					Annotations: map[string]string{
						"k8s.v1.cni.cncf.io/networks": "mynet",
					},
				},
			},
			expected: false,
		},
		{
			name: "pod with networks and matching network-status with IP - valid",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-valid-multus",
					Annotations: map[string]string{
						"k8s.v1.cni.cncf.io/networks":        "mynet",
						"k8s.v1.cni.cncf.io/network-status": `[{"name":"mynet","interface":"net1","ips":["10.0.0.2"]}]`,
					},
				},
			},
			expected: true,
		},
		{
			name: "pod with networks but network-status missing that network - invalid",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-wrong-network",
					Annotations: map[string]string{
						"k8s.v1.cni.cncf.io/networks":        "mynet",
						"k8s.v1.cni.cncf.io/network-status": `[{"name":"othernet","interface":"net1","ips":["10.0.0.2"]}]`,
					},
				},
			},
			expected: false,
		},
		{
			name: "pod with networks and network-status but no IP - invalid",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-no-ip",
					Annotations: map[string]string{
						"k8s.v1.cni.cncf.io/networks":        "mynet",
						"k8s.v1.cni.cncf.io/network-status": `[{"name":"mynet","interface":"net1","ips":[]}]`,
					},
				},
			},
			expected: false,
		},
		{
			name: "pod with multiple networks all valid - valid",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-multi-valid",
					Annotations: map[string]string{
						"k8s.v1.cni.cncf.io/networks":        "net1,net2",
						"k8s.v1.cni.cncf.io/network-status": `[{"name":"net1","interface":"net1","ips":["10.0.0.1"]},{"name":"net2","interface":"net2","ips":["10.0.0.2"]}]`,
					},
				},
			},
			expected: true,
		},
		{
			name: "pod with multiple networks but one missing - invalid",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-multi-invalid",
					Annotations: map[string]string{
						"k8s.v1.cni.cncf.io/networks":        "net1,net2",
						"k8s.v1.cni.cncf.io/network-status": `[{"name":"net1","interface":"net1","ips":["10.0.0.1"]}]`,
					},
				},
			},
			expected: false,
		},
		{
			name: "pod with JSON array networks format - valid",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-json-format",
					Annotations: map[string]string{
						"k8s.v1.cni.cncf.io/networks":        `[{"name":"custom-net"}]`,
						"k8s.v1.cni.cncf.io/network-status": `[{"name":"custom-net","interface":"net1","ips":["10.0.0.3"]}]`,
					},
				},
			},
			expected: true,
		},
		{
			name: "pod with namespace-qualified network name in status - valid",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-namespace-qualified",
					Namespace: "test-ns",
					Annotations: map[string]string{
						"k8s.v1.cni.cncf.io/networks":        "aeron",
						"k8s.v1.cni.cncf.io/network-status": `[{"name":"test-ns/aeron","interface":"net1","ips":["192.168.1.201"]}]`,
					},
				},
			},
			expected: true,
		},
		{
			name: "pod with wrong namespace in qualified network name - invalid",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-wrong-namespace",
					Namespace: "test-ns",
					Annotations: map[string]string{
						"k8s.v1.cni.cncf.io/networks":        "aeron",
						"k8s.v1.cni.cncf.io/network-status": `[{"name":"other-ns/aeron","interface":"net1","ips":["192.168.1.201"]}]`,
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateMultusNetworkStatus(tt.pod)
			if result != tt.expected {
				t.Errorf("validateMultusNetworkStatus() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGetMediaDriverPodsWithMultusValidation(t *testing.T) {
	tests := []struct {
		name     string
		pods     []corev1.Pod
		expected int
	}{
		{
			name: "pods without multus annotations included",
			pods: []corev1.Pod{
				createTestPod("aeron-1", "10.0.0.1", "Running", time.Now().Add(-5*time.Minute)),
			},
			expected: 1,
		},
		{
			name: "pods with valid multus annotations included",
			pods: []corev1.Pod{
				createTestPodWithMultus("aeron-1", "10.0.0.1", "mynet", "10.0.0.2", time.Now().Add(-5*time.Minute)),
			},
			expected: 1,
		},
		{
			name: "pods with invalid multus annotations excluded",
			pods: []corev1.Pod{
				createTestPodWithInvalidMultus("aeron-1", "10.0.0.1", time.Now().Add(-5*time.Minute)),
			},
			expected: 0,
		},
		{
			name: "mix of valid and invalid multus pods",
			pods: []corev1.Pod{
				createTestPod("aeron-no-multus", "10.0.0.1", "Running", time.Now().Add(-10*time.Minute)),
				createTestPodWithMultus("aeron-valid-multus", "10.0.0.2", "mynet", "10.0.0.3", time.Now().Add(-5*time.Minute)),
				createTestPodWithInvalidMultus("aeron-invalid-multus", "10.0.0.4", time.Now().Add(-3*time.Minute)),
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()

			for _, pod := range tt.pods {
				_, err := clientset.CoreV1().Pods("test-namespace").Create(context.TODO(), &pod, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Failed to create test pod: %v", err)
				}
			}

			result, err := getMediaDriverPods(clientset, "test-namespace", "aeron.io/media-driver=true", 0)
			if err != nil {
				t.Fatalf("getMediaDriverPods() error = %v", err)
			}

			if len(result) != tt.expected {
				t.Errorf("Expected %d pods, got %d", tt.expected, len(result))
			}
		})
	}
}

// Helper functions for creating test pods
func createTestPod(name, ip, phase string, creationTime time.Time) corev1.Pod {
	return createTestPodWithLabel(name, ip, phase, creationTime, "aeron.io/media-driver", "true")
}

func createTestPodWithLabel(name, ip, phase string, creationTime time.Time, labelKey, labelValue string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				labelKey: labelValue,
			},
			CreationTimestamp: metav1.NewTime(creationTime),
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPhase(phase),
			PodIP: ip,
		},
	}
}

func createTestPodWithoutIP(name, phase string, creationTime time.Time) corev1.Pod {
	pod := createTestPodWithLabel(name, "", phase, creationTime, "aeron.io/media-driver", "true")
	pod.Status.PodIP = "" // Explicitly set no IP address
	return pod
}

func createTestPodWithSecondaryInterface(name, primaryIP, secondaryIP, phase string, networkName string, interfaceName string, creationTime time.Time) corev1.Pod {

	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"aeron.io/media-driver": "true",
			},
			CreationTimestamp: metav1.NewTime(creationTime),
			Annotations: map[string]string{
				"k8s.v1.cni.cncf.io/network-status": fmt.Sprintf("[{\"name\":\"aws-cni\",\"interface\":\"eth0\",\"ips\":[\"10.190.223.111\"],\"default\":true,\"dns\":{}},{\"name\":\"aws-cni\",\"interface\":\"dummy9e99c8bc34f\",\"mac\":\"0\",\"dns\":{}},{\"name\":\"%s\",\"interface\":\"%s\",\"ips\":[\"%s\"],\"mac\":\"02:4a:ef:75:4e:00\",\"dns\":{}}]", networkName, interfaceName, secondaryIP),
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPhase(phase),
			PodIP: primaryIP,
		},
	}
}

func createTestPodWithMultus(name, primaryIP, networkName, secondaryIP string, creationTime time.Time) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"aeron.io/media-driver": "true",
			},
			CreationTimestamp: metav1.NewTime(creationTime),
			Annotations: map[string]string{
				"k8s.v1.cni.cncf.io/networks":       networkName,
				"k8s.v1.cni.cncf.io/network-status": fmt.Sprintf(`[{"name":"%s","interface":"net1","ips":["%s"]}]`, networkName, secondaryIP),
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: primaryIP,
		},
	}
}

func createTestPodWithInvalidMultus(name, primaryIP string, creationTime time.Time) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"aeron.io/media-driver": "true",
			},
			CreationTimestamp: metav1.NewTime(creationTime),
			Annotations: map[string]string{
				"k8s.v1.cni.cncf.io/networks": "mynet",
				// Missing network-status annotation - this makes it invalid
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: primaryIP,
		},
	}
}
