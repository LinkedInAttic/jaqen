/*
 * Copyright 2017 LinkedIn Corporation. All rights reserved. Licensed under the BSD-2 Clause license.
 * See LICENSE in the project root for license information.
 */

package main

import (
	"net"
)

// Address is a helper class for dealing with IPs.
// It understands IPv4/IPv6, CNAMEs and mappings of internal/external IPs
type Address struct {
	Port       string // Port is the port in the pair (if specified)
	Host       string // Host is the hostname of the address (ex. CNAMEs)
	InternalIP net.IP // internal is the parsed ip address for internal use (binding/iptables)
	ExternalIP net.IP // external is the parsed ip address for external use (dns)
}

// NewAddress creates a *Address instance
func NewAddress(rawAddr string) *Address {
	host, port, err := net.SplitHostPort(rawAddr)
	if err != nil {
		// TODO: Hard-coded error message
		if err.(*net.AddrError).Err == "missing port in address" {
			rawAddr = rawAddr + ":80" // Default to port 80 if not specified
			host, port, err = net.SplitHostPort(rawAddr)
		} else if err != nil {
			log.Errorf(`Failed to parse address "%s": %v`, rawAddr, err)
			return nil
		}
	}
	addr := &Address{
		Port: port,
		Host: host,
	}
	ip := net.ParseIP(host)
	if ip != nil {
		addr.InternalIP = ip
		addr.ExternalIP = ip // We assume external == internal on init, can be overwritten later
	}
	return addr
}

// InternalAddr returns InternalIP and the port
func (a *Address) InternalAddr() string {
	if a.InternalIP.To4() == nil {
		return "[" + a.InternalIP.String() + "]:" + a.Port
	}
	return a.InternalIP.String() + ":" + a.Port
}

// ExternalAddr returns ExternalIP and the port
func (a *Address) ExternalAddr() string {
	if a.ExternalIP.To4() == nil {
		return "[" + a.ExternalIP.String() + "]:" + a.Port
	}
	return a.ExternalIP.String() + ":" + a.Port
}

// IP returns InternalIP (alias)
func (a *Address) IP() net.IP {
	return a.InternalIP
}

// IsUnknownHost returns true if the address contains a host, not an IP
func (a *Address) IsUnknownHost() bool {
	return a.Host != "" && a.InternalIP == nil
}

// UnmarshalJSON just calls NewAddress
func (a *Address) UnmarshalJSON(b []byte) error {
	addr := NewAddress(string(b[1 : len(b)-1]))
	a.Port = addr.Port
	a.Host = addr.Host
	a.InternalIP = addr.InternalIP
	a.ExternalIP = addr.ExternalIP
	return nil
}

// String can be used for maps
func (a *Address) String() string {
	if a.IsUnknownHost() {
		return a.Host + ":" + a.Port
	} else {
		return a.ExternalIP.String() + "\\" + a.InternalIP.String() + ":" + a.Port
	}
}

// Equals can be used to compare Address instances
func (a *Address) Equal(c *Address) bool {
	if a.Port != c.Port {
		return false
	}
	if !a.InternalIP.Equal(c.InternalIP) {
		return false
	}
	if !a.ExternalIP.Equal(c.ExternalIP) {
		return false
	}
	return true
}

// Clone will create an identical *Address instance
func (a *Address) Clone() *Address {
	return &Address{
		Port:       a.Port,
		Host:       a.Host,
		InternalIP: a.InternalIP,
		ExternalIP: a.ExternalIP,
	}
}
