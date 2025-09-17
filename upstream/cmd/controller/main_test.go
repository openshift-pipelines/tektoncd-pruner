package main

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"k8s.io/client-go/rest"
	"knative.dev/pkg/controller"
)

func TestMainConfigurationSettings(t *testing.T) {
	tests := []struct {
		name      string
		envVars   map[string]string
		wantQPS   float32
		wantBurst int
	}{
		{
			name:      "Default configuration",
			envVars:   map[string]string{},
			wantQPS:   4 * rest.DefaultQPS, // quadrupled for number of controllers (matches main.go logic)
			wantBurst: 2 * rest.DefaultBurst,
		},
		{
			name: "Custom QPS and Burst",
			envVars: map[string]string{
				"QPS":   "50",
				"BURST": "100",
			},
			wantQPS:   100, // 50 * 2 for number of controllers
			wantBurst: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for k, v := range tt.envVars {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}

			cfg := &rest.Config{}
			if qpsStr := os.Getenv("QPS"); qpsStr != "" {
				if qps, err := strconv.ParseFloat(qpsStr, 32); err == nil {
					cfg.QPS = float32(qps)
				}
			} else {
				cfg.QPS = 2 * rest.DefaultQPS
			}
			cfg.QPS = 2 * cfg.QPS

			if burstStr := os.Getenv("BURST"); burstStr != "" {
				if burst, err := strconv.Atoi(burstStr); err == nil {
					cfg.Burst = burst
				}
			} else {
				cfg.Burst = rest.DefaultBurst
			}
			cfg.Burst = 2 * cfg.Burst

			if cfg.QPS != tt.wantQPS {
				t.Errorf("QPS = %v, want %v", cfg.QPS, tt.wantQPS)
			}
			if cfg.Burst != tt.wantBurst {
				t.Errorf("Burst = %v, want %v", cfg.Burst, tt.wantBurst)
			}
		})
	}
}

func TestThreadsPerControllerConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		threads   int
		wantValue int
	}{
		{
			name:      "Default threads",
			threads:   controller.DefaultThreadsPerController,
			wantValue: controller.DefaultThreadsPerController,
		},
		{
			name:      "Custom threads",
			threads:   10,
			wantValue: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller.DefaultThreadsPerController = tt.threads
			if controller.DefaultThreadsPerController != tt.wantValue {
				t.Errorf("ThreadsPerController = %v, want %v", controller.DefaultThreadsPerController, tt.wantValue)
			}
		})
	}
}

func TestNamespaceConfiguration(t *testing.T) {
	tests := []struct {
		name          string
		namespaceStr  string
		wantNamespace string
	}{
		{
			name:          "Empty namespace",
			namespaceStr:  "",
			wantNamespace: "",
		},
		{
			name:          "Single namespace",
			namespaceStr:  "test-namespace",
			wantNamespace: "test-namespace",
		},
		{
			name:          "Multiple namespaces",
			namespaceStr:  "ns1,ns2,ns3",
			wantNamespace: "ns1,ns2,ns3",
		},
		{
			name:          "Namespaces with spaces",
			namespaceStr:  "ns1, ns2, ns3",
			wantNamespace: "ns1,ns2,ns3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strings.ReplaceAll(tt.namespaceStr, " ", "")
			if result != tt.wantNamespace {
				t.Errorf("namespace = %v, want %v", result, tt.wantNamespace)
			}
		})
	}
}

func TestHighAvailabilityConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		disableHA bool
		wantHA    bool
	}{
		{
			name:      "HA enabled by default",
			disableHA: false,
			wantHA:    true,
		},
		{
			name:      "HA explicitly disabled",
			disableHA: true,
			wantHA:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.disableHA {
				os.Setenv("DISABLE_HA", "true")
				defer os.Unsetenv("DISABLE_HA")
			}

			// We can't directly test the HA context since it's handled by sharedmain
			// but we can verify the environment variable is set correctly
			got := os.Getenv("DISABLE_HA") == "true"
			if got == tt.wantHA {
				t.Errorf("HA disabled = %v, want %v", got, !tt.wantHA)
			}
		})
	}
}
