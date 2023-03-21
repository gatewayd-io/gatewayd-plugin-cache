package plugin

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/zenizh/go-capturer"
)

// Test_Plugin_getProxies_Fails_APIAddress tests that getProxies() fails when APIAddress is not set.
func Test_Plugin_getProxies_Fails_APIAddress(t *testing.T) {
	output := capturer.CaptureOutput(func() {
		p := Plugin{
			Logger: hclog.New(&hclog.LoggerOptions{
				Name:   "test",
				Level:  hclog.Trace,
				Output: os.Stdout,
			}),
		}
		proxies := p.getProxies()
		assert.Nil(t, proxies)
	})

	assert.Contains(t, output, "[ERROR] test: Failed to get a list of proxies from GatewayD: error=\"APIAddress is not set\"\n")
}

// Test_Plugin_getProxies_Fails_Request_Error tests that getProxies() fails when the request fails.
func Test_Plugin_getProxies_Fails_Request_Error(t *testing.T) {
	output := capturer.CaptureOutput(func() {
		p := Plugin{
			Logger: hclog.New(&hclog.LoggerOptions{
				Name:   "test",
				Level:  hclog.Trace,
				Output: os.Stdout,
			}),
			APIAddress: "localhost:12345",
		}
		proxies := p.getProxies()
		assert.Nil(t, proxies)
	})

	assert.Contains(t, output, "[ERROR] test: Failed to get a list of proxies from GatewayD: error=\"Get \\\"http://localhost:12345/v1/GatewayDPluginService/GetProxies\\\": dial tcp [::1]:12345: connect: connection refused\"\n")
}

// Test_Plugin_getProxies_Fails_Inaccessible tests that getProxies() fails
// when the API is not accessible.
func Test_Plugin_getProxies_Fails_Inaccessible(t *testing.T) {
	output := capturer.CaptureOutput(func() {
		p := Plugin{
			Logger: hclog.New(&hclog.LoggerOptions{
				Name:   "test",
				Level:  hclog.Trace,
				Output: os.Stdout,
			}),
			APIAddress: "127.0.0.1:18080",
		}
		proxies := p.getProxies()
		assert.Nil(t, proxies)
	})

	assert.Contains(t, output, "[ERROR] test: Failed to get a list of proxies from GatewayD: error=\"Get \\\"http://127.0.0.1:18080/v1/GatewayDPluginService/GetProxies\\\": dial tcp 127.0.0.1:18080: connect: connection refused\"\n")
}

// Test_Plugin_getProxies_Fails_Invalid_JSON tests that getProxies() fails
// when the response is not valid JSON.
func Test_Plugin_getProxies_Fails_Invalid_JSON(t *testing.T) {
	output := capturer.CaptureOutput(func() {
		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(``)); err != nil {
				t.Fatal(err)
			}
		}))
		defer testServer.Close()

		p := Plugin{
			Logger: hclog.New(&hclog.LoggerOptions{
				Name:   "test",
				Level:  hclog.Trace,
				Output: os.Stdout,
			}),
			APIAddress: strings.TrimPrefix(testServer.URL, "http://"),
		}

		proxies := p.getProxies()
		assert.Nil(t, proxies)
	})

	assert.Contains(t, output, "[ERROR] test: Failed to decode response from GatewayD: error=EOF\n")
}

// Test_Plugin_getProxies_Fails_Empty_Response tests that getProxies() returns an empty map
// when the response is empty.
func Test_Plugin_getProxies_Fails_Empty_Response(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{}`)); err != nil {
			t.Fatal(err)
		}
	}))
	defer testServer.Close()

	p := Plugin{
		Logger: hclog.New(&hclog.LoggerOptions{
			Name:   "test",
			Level:  hclog.Trace,
			Output: os.Stdout,
		}),
		APIAddress: strings.TrimPrefix(testServer.URL, "http://"),
	}

	proxies := p.getProxies()
	assert.NotNil(t, proxies)
	assert.Equal(t, 0, len(proxies))
	assert.Equal(t, proxies, map[string]Proxy{})
}

// Test_Plugin_getProxies tests that getProxies() returns a map of proxies.
func Test_Plugin_getProxies(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"default":{"available":["localhost:45121"],"busy":["localhost:45123"],"total":2}}`)); err != nil {
			t.Fatal(err)
		}
	}))
	defer testServer.Close()

	p := Plugin{
		Logger: hclog.New(&hclog.LoggerOptions{
			Name:   "test",
			Level:  hclog.Trace,
			Output: os.Stdout,
		}),
		APIAddress: strings.TrimPrefix(testServer.URL, "http://"),
	}

	proxies := p.getProxies()
	assert.Equal(t, 1, len(proxies))
	assert.Equal(t, 1, len(proxies["default"].Available))
	assert.Equal(t, 1, len(proxies["default"].Busy))
	assert.Equal(t, 2, proxies["default"].Total)
	assert.Equal(t, "localhost:45121", proxies["default"].Available[0])
	assert.Equal(t, "localhost:45123", proxies["default"].Busy[0])
}
