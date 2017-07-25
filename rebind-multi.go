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

// MultiRecordRebind is a rebind that occurs instantly, relying on some connection trickery to make work
type MultiRecordRebind struct {
	target   *Address
	ttl      uint32
	v4Server *HTTPServer
	v6Server *HTTPServer
}

// NewMultiRecordRebind creates a *MultiRecordRebind instance, leasing servers as required
func NewMultiRecordRebind(ctx context.Context, m *RebindManager, target *Address, ttl uint32) (r *MultiRecordRebind) {
	r = &MultiRecordRebind{
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
func (r *MultiRecordRebind) HandleDNS(qType uint16) (ans []DNSAnswer) {
	if qType == dns.TypeAAAA {
		if r.v6Server != nil && r.target.IP().To4() == nil {
			ans = []DNSAnswer{
				DNSAnswer{
					TTL:     r.ttl,
					Address: r.v6Server.Address,
				},
				DNSAnswer{
					TTL:     r.ttl,
					Address: r.target,
				},
			}
		}
	} else {
		if r.v4Server != nil && r.target.IP().To4() != nil {
			ans = []DNSAnswer{
				DNSAnswer{
					TTL:     r.ttl,
					Address: r.v4Server.Address,
				},
				DNSAnswer{
					TTL:     r.ttl,
					Address: r.target,
				},
			}
		}
	}
	return
}

// HTTP Middleware implements the banning logic
func (r *MultiRecordRebind) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Debugf(r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
