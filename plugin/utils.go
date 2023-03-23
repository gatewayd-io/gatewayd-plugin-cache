package plugin

import (
	"net"
	"strconv"
	"strings"
)

const AddressPortPairLength = 2

// validateIP checks if an IP address is valid.
func validateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}

	if ip.To4() != nil || ip.To16() != nil {
		return true
	}

	return false
}

// validateAddressPort validates an address:port string.
func validateAddressPort(addressPort string) (bool, error) {
	// Split the address and port.
	// TODO: Support IPv6.
	data := strings.Split(strings.TrimSpace(addressPort), ":")
	if len(data) != AddressPortPairLength {
		return false, ErrInvalidAddressPortPair
	}

	// Validate the port.
	port, err := strconv.ParseUint(data[1], 10, 16)
	if err != nil {
		return false, err
	}

	// Resolve the IP address, if it is a host.
	ipAddress, err := net.ResolveIPAddr("ip", data[0])
	if err != nil {
		return false, err
	}

	// Validate the IP address and port.
	if (validateIP(net.ParseIP(data[0])) || validateIP(ipAddress.IP)) && (port > 0 && port <= 65535) {
		return true, nil
	}

	return false, nil
}

// validateHostPort validates a host:port string.
// TODO: Add support for IPv6.
func validateHostPort(hostPort string) (bool, error) {
	data := strings.Split(hostPort, ":")
	if len(data) != AddressPortPairLength {
		return false, ErrInvalidAddressPortPair
	}

	port, err := strconv.ParseUint(data[1], 10, 16)
	if err != nil {
		return false, err
	}

	// FIXME: There is not much to validate on the host side.
	if data[0] != "" && port > 0 && port <= 65535 {
		return true, nil
	}

	return false, nil
}

// isBusy checks if a client address exists in cache by matching the address
// with the busy clients.
func isBusy(proxies map[string]Proxy, address string) bool {
	if proxies == nil {
		// NOTE: If the API is not running, we assume that the client is busy,
		// so that we don't accidentally make the client and the plugin unstable.
		return true
	}

	for _, name := range proxies {
		for _, client := range name.Busy {
			if client == address {
				return true
			}
		}
	}
	return false
}
