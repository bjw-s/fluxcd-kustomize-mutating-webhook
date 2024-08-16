package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestMutatingWebhook(t *testing.T) {
	// Set up test config
	appConfig.config = map[string]string{
		"TEST_KEY": "test_value",
	}

	tests := []struct {
		name            string
		inputObject     map[string]interface{}
		kind            metav1.GroupVersionKind
		expectedPatch   []map[string]interface{}
		expectedAllowed bool
	}{
		{
			name: "Add postBuild and substitute",
			inputObject: map[string]interface{}{
				"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
				"kind":       "Kustomization",
				"metadata": map[string]interface{}{
					"name":      "test-kustomization",
					"namespace": "default",
				},
				"spec": map[string]interface{}{},
			},
			kind: metav1.GroupVersionKind{
				Group:   "kustomize.toolkit.fluxcd.io",
				Version: "v1",
				Kind:    "Kustomization",
			},
			expectedPatch: []map[string]interface{}{
				{
					"op":    "add",
					"path":  "/spec/postBuild",
					"value": map[string]interface{}{},
				},
				{
					"op":    "add",
					"path":  "/spec/postBuild/substitute",
					"value": map[string]interface{}{},
				},
				{
					"op":    "add",
					"path":  "/spec/postBuild/substitute/TEST_KEY",
					"value": "test_value",
				},
			},
			expectedAllowed: true,
		},
		{
			name: "No mutation for non-Kustomization resource",
			inputObject: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "test-configmap",
					"namespace": "default",
				},
				"data": map[string]interface{}{},
			},
			kind: metav1.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "ConfigMap",
			},
			expectedPatch:   nil,
			expectedAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create admission review request
			objBytes, err := json.Marshal(tt.inputObject)
			require.NoError(t, err)

			ar := admissionv1.AdmissionReview{
				Request: &admissionv1.AdmissionRequest{
					Object:    runtime.RawExtension{Raw: objBytes},
					Kind:      tt.kind,
					Operation: admissionv1.Create,
				},
			}

			arBytes, err := json.Marshal(ar)
			require.NoError(t, err)

			// Create request
			req, err := http.NewRequest("POST", "/mutate", bytes.NewBuffer(arBytes))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			// Create response recorder
			rr := httptest.NewRecorder()

			// Call the handler
			handleMutate(rr, req)

			// Check the status code
			assert.Equal(t, http.StatusOK, rr.Code)

			// Parse the response
			var respAR admissionv1.AdmissionReview
			err = json.Unmarshal(rr.Body.Bytes(), &respAR)
			require.NoError(t, err)

			// Check the response
			assert.Equal(t, tt.expectedAllowed, respAR.Response.Allowed)

			if tt.expectedPatch != nil {
				var patch []map[string]interface{}
				err = json.Unmarshal(respAR.Response.Patch, &patch)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedPatch, patch)
			} else {
				assert.Nil(t, respAR.Response.Patch)
			}

			t.Logf("Test case: %s", tt.name)
			t.Logf("Input object: %v", tt.inputObject)
			t.Logf("Response: %v", respAR.Response)
		})
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectedErr string
	}{
		{
			name: "Valid configuration",
			config: Config{
				ServerAddress: ":8443",
				CertFile:      "/path/to/cert",
				KeyFile:       "/path/to/key",
				ConfigDir:     "/path/to/config",
				LogLevel:      "info",
				RateLimit:     100,
			},
			expectedErr: "",
		},
		{
			name: "Missing server address",
			config: Config{
				CertFile:  "/path/to/cert",
				KeyFile:   "/path/to/key",
				ConfigDir: "/path/to/config",
				LogLevel:  "info",
				RateLimit: 100,
			},
			expectedErr: "server address is required",
		},
		{
			name: "Invalid rate limit",
			config: Config{
				ServerAddress: ":8443",
				CertFile:      "/path/to/cert",
				KeyFile:       "/path/to/key",
				ConfigDir:     "/path/to/config",
				LogLevel:      "info",
				RateLimit:     0,
			},
			expectedErr: "rate limit must be greater than 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)
			if tt.expectedErr == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.expectedErr)
			}
		})
	}
}

func TestGetAppConfig(t *testing.T) {
	appConfig.config = map[string]string{
		"KEY1": "value1",
		"KEY2": "value2",
	}

	tests := []struct {
		name          string
		key           string
		expectedValue string
		expectedOk    bool
	}{
		{
			name:          "Existing key",
			key:           "KEY1",
			expectedValue: "value1",
			expectedOk:    true,
		},
		{
			name:          "Non-existing key",
			key:           "KEY3",
			expectedValue: "",
			expectedOk:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := getAppConfig(tt.key)
			assert.Equal(t, tt.expectedValue, value)
			assert.Equal(t, tt.expectedOk, ok)
		})
	}
}

func TestLoadConfig(t *testing.T) {
	// Save the original environment
	origEnv := os.Environ()
	defer func() {
		// Restore the original environment after the test
		os.Clearenv()
		for _, env := range origEnv {
			pair := strings.SplitN(env, "=", 2)
			os.Setenv(pair[0], pair[1])
		}
	}()

	// Set test environment variables
	os.Setenv("SERVER_ADDRESS", ":9443")
	os.Setenv("CERT_FILE", "/custom/cert/path")
	os.Setenv("RATE_LIMIT", "200")

	config := loadConfig()

	assert.Equal(t, ":9443", config.ServerAddress)
	assert.Equal(t, "/custom/cert/path", config.CertFile)
	assert.Equal(t, defaultKeyFile, config.KeyFile)
	assert.Equal(t, defaultConfigDir, config.ConfigDir)
	assert.Equal(t, defaultLogLevel, config.LogLevel)
	assert.Equal(t, 200, config.RateLimit)
}

func TestEscapeJsonPointer(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "No special characters",
			input:    "normal/path",
			expected: "normal~1path",
		},
		{
			name:     "With tilde",
			input:    "path/with~tilde",
			expected: "path~1with~0tilde",
		},
		{
			name:     "With forward slash",
			input:    "path/with/slash",
			expected: "path~1with~1slash",
		},
		{
			name:     "With both tilde and slash",
			input:    "path/with~/and/",
			expected: "path~1with~0~1and~1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeJsonPointer(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func BenchmarkMutatingWebhook(b *testing.B) {
	// Set up test config
	appConfig.config = map[string]string{
		"TEST_KEY": "test_value",
	}

	inputObject := map[string]interface{}{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata": map[string]interface{}{
			"name":      "test-kustomization",
			"namespace": "default",
		},
		"spec": map[string]interface{}{},
	}

	objBytes, _ := json.Marshal(inputObject)

	ar := admissionv1.AdmissionReview{
		Request: &admissionv1.AdmissionRequest{
			Object: runtime.RawExtension{Raw: objBytes},
			Kind: metav1.GroupVersionKind{
				Group:   "kustomize.toolkit.fluxcd.io",
				Version: "v1",
				Kind:    "Kustomization",
			},
			Operation: admissionv1.Create,
		},
	}

	arBytes, _ := json.Marshal(ar)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("POST", "/mutate", bytes.NewBuffer(arBytes))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		handleMutate(rr, req)
	}
}
