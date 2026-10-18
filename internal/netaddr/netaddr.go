// SPDX-License-Identifier: Apache-2.0

// Package netaddr decides where the HTTP servers listen and whether that is reachable from other hosts.
package netaddr

import "net"

// Listen returns the address to listen on and whether it is reachable from other hosts.
// An empty host means all interfaces and therefore counts as exposed.
func Listen(host, port string) (addr string, exposed bool) {
	return net.JoinHostPort(host, port), !IsLoopback(host)
}

// IsLoopback reports whether host is "localhost" or a loopback IP (127.0.0.0/8, ::1).
// Other hostnames are treated as exposed; no DNS lookup is done.
func IsLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
