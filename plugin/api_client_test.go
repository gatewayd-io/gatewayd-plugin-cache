package plugin

import (
	"os"
	"testing"

	apiV1 "github.com/gatewayd-io/gatewayd/api/v1"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/zenizh/go-capturer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Test_Plugin_getProxies_Fails_APIClient tests that getProxies()
// fails when the API client is not initialized.
func Test_Plugin_getProxies_Fails_APIClient(t *testing.T) {
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

	assert.Contains(t, output, "[ERROR] test: Failed to get a list of proxies from GatewayD: error=\"API client is not initialized\"\n")
}

// Test_Plugin_getProxies_Fails_Connection_Error tests that getProxies()
// fails when the API client fails to connect to GatewayD.
func Test_Plugin_getProxies_Fails_Connection_Error(t *testing.T) {
	output := capturer.CaptureOutput(func() {
		conn, err := grpc.NewClient(
			"localhost:18080",
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		assert.NoError(t, err)
		p := Plugin{
			Logger: hclog.New(&hclog.LoggerOptions{
				Name:   "test",
				Level:  hclog.Trace,
				Output: os.Stdout,
			}),
			APIClient: apiV1.NewGatewayDAdminAPIServiceClient(conn),
		}
		proxies := p.getProxies()
		assert.Nil(t, proxies)
	})

	assert.Contains(t, output, `[ERROR] test: Failed to get a list of proxies from GatewayD`)
	assert.Contains(t, output, `connection refused`)
}
