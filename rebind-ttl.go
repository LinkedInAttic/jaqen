/*
 * Copyright 2017 LinkedIn Corporation. All rights reserved. Licensed under the BSD-2 Clause license.
 * See LICENSE in the project root for license information.
 */

package main

import (
	"context"
	"net/http"

	"github.com/miekg/dns"
)

// TTLRebind is a generic rebind
type TTLRebind struct {
	target   *Address
	ttl      uint32
	rebind   bool
	v4Server *HTTPServer
	v6Server *HTTPServer
}

// NewTTLRebind creates a *TTLRebind instance, leasing servers as required
func NewTTLRebind(ctx context.Context, m *RebindManager, target *Address, ttl uint32) (r *TTLRebind) {
	r = &TTLRebind{
		target: target,
		ttl:    ttl,
	}
	// If we can't parse out an IP, must be a CNAME rebind, we need 2 servers IPv4 and IPv6 since we don't know the family of the CNAME target
	if target.IP() == nil {
		r.v4Server = m.GetHTTPServer(ctx, m.pool.Lease(ctx, &PoolCriteriaAddressFamily{IPv6: false}), target)
		r.v6Server = m.GetHTTPServer(ctx, m.pool.Lease(ctx, &PoolCriteriaAddressFamily{IPv6: true}), target)
		// IPv6
	} else if target.IP().To4() == nil {
		r.v6Server = m.GetHTTPServer(ctx, m.pool.Lease(ctx, &PoolCriteriaAddressFamily{IPv6: true}), target)
		// IPv4
	} else {
		r.v4Server = m.GetHTTPServer(ctx, m.pool.Lease(ctx, &PoolCriteriaAddressFamily{IPv6: false}), target)
	}
	return
}

// HandleDNS handles DNS requests
func (r *TTLRebind) HandleDNS(qType uint16) (ans []DNSAnswer) {
	// If this is the first request, rebind on next
	if r.rebind == false {
		r.rebind = true
		if qType == dns.TypeAAAA {
			if r.v6Server != nil {
				ans = []DNSAnswer{
					DNSAnswer{
						TTL:     r.ttl,
						Address: r.v6Server.Address,
					},
				}
			}
		} else {
			if r.v4Server != nil {
				ans = []DNSAnswer{
					DNSAnswer{
						TTL:     r.ttl,
						Address: r.v4Server.Address,
					},
				}
			}
		}
	} else {
		if (r.target.IP().To4() == nil && qType == dns.TypeAAAA) || (r.target.IP().To4() != nil && qType == dns.TypeA) {
			ans = []DNSAnswer{
				DNSAnswer{
					TTL:     r.ttl,
					Address: r.target,
				},
			}
		}
	}
	return
}

// HTTPMiddleware is a NOP for this use-case
func (r *TTLRebind) HTTPMiddleware(next http.Handler) http.Handler {
	return next
}
