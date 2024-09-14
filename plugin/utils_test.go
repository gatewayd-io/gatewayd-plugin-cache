package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_validateAddressPort_Hostname(t *testing.T) {
	valid, err := validateAddressPort("localhost:5432")
	assert.True(t, valid)
	assert.Nil(t, err)

	valid, err = validateAddressPort("	localhost:5432")
	assert.True(t, valid)
	assert.Nil(t, err)

	valid, err = validateAddressPort("localhost:5432	")
	assert.True(t, valid)
	assert.Nil(t, err)

	valid, err = validateAddressPort("	localhost:5432	")
	assert.True(t, valid)
	assert.Nil(t, err)
}

func Test_validateAddressPort_Fails(t *testing.T) {
	valid, err := validateAddressPort("localhost")
	assert.False(t, valid)
	assert.NotNil(t, err)
}

func Test_validateHostPort_Fails(t *testing.T) {
	valid, err := validateHostPort("127.0.0.1")
	assert.False(t, valid)
	assert.NotNil(t, err)
}

func Test_validateAddressPort_IPv4(t *testing.T) {
	valid, err := validateAddressPort("127.0.0.1:5432")
	assert.True(t, valid)
	assert.Nil(t, err)

	valid, err = validateAddressPort("    127.0.0.1:5432")
	assert.True(t, valid)
	assert.Nil(t, err)

	valid, err = validateAddressPort("127.0.0.1:5432  ")
	assert.True(t, valid)
	assert.Nil(t, err)

	valid, err = validateAddressPort("    127.0.0.1:5432  ")
	assert.True(t, valid)
	assert.Nil(t, err)
}

func Test_isBusy(t *testing.T) {
	proxies := map[string]map[string]Proxy{
		"default": {
			"reads": {
				Busy: []string{"localhost:12345"},
			},
		},
	}
	assert.True(t, isBusy(proxies, "localhost:12345"))
}

func Test_isBusy_False(t *testing.T) {
	proxies := map[string]map[string]Proxy{
		"default": {
			"reads": {
				Busy: []string{"localhost:12345"},
			},
		},
	}
	assert.False(t, isBusy(proxies, "localhost:54321"))
}

func Test_isBusy_False_Empty(t *testing.T) {
	proxies := map[string]map[string]Proxy{
		"default": {
			"reads": {
				Busy: []string{},
			},
		},
	}
	assert.False(t, isBusy(proxies, "localhost:54321"))
}

func Test_isBusy_False_EmptyMap(t *testing.T) {
	proxies := map[string]map[string]Proxy{}
	assert.False(t, isBusy(proxies, "localhost:54321"))
}
