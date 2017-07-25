/*
 * Copyright 2017 LinkedIn Corporation. All rights reserved. Licensed under the BSD-2 Clause license.
 * See LICENSE in the project root for license information.
 */

package main

import (
	"context"
	"math/rand"
	"sync"
)

// Pool represents a pool of addresses to use for binding to HTTP ports
type Pool struct {
	mutex  *sync.Mutex                  // Avoid and race-conditions by just using a mutex TODO: determine performance impact
	addrs  []*Address                   // Available addresses
	leases map[string][]context.Context // Leases to a given context by IP TODO: can't use net.IP as key, this is potentially performance impacting
}

// NewPool creates a *Pool instance given a list of available addresses
func NewPool(addrs []*Address) *Pool {
	leases := make(map[string][]context.Context)
	for _, addr := range addrs {
		leases[addr.String()] = []context.Context{}
	}
	return &Pool{
		mutex:  new(sync.Mutex),
		addrs:  addrs,
		leases: leases,
	}
}

// PoolCriteria defines a interface that decides if a given IP is eligible
type PoolCriteria interface {
	Eligible([]context.Context, *Address) bool
}

// PoolCriteriaAddressFamily matches addresses which are in the same address family (IPv4/IPv6)
type PoolCriteriaAddressFamily struct {
	IPv6 bool
}

// Eligible will only return true if the address is in the family specified earlier
func (c *PoolCriteriaAddressFamily) Eligible(ctxs []context.Context, addr *Address) bool {
	return (addr.IP().To4() == nil) == c.IPv6
}

// PoolCriteriaExactMatch matches addresses which are exactly the address provided
type PoolCriteriaExternalIPMatch struct {
	Addr *Address
}

// Eligible will only return true if the address exactly matches
func (c *PoolCriteriaExternalIPMatch) Eligible(ctxs []context.Context, addr *Address) bool {
	return addr.ExternalIP.Equal(c.Addr.ExternalIP)
}

// Lease will attempt to "lease" an address that meets "criteria" for the duration of context, releasing it back into the pool when the context is cancelled
func (p *Pool) Lease(ctx context.Context, criteriaList ...PoolCriteria) *Address {
	// Obtain lock
	p.mutex.Lock()
	defer p.mutex.Unlock()
	// Loop each address to determine eligibility
	var eligibleAddrs []*Address
	for _, addr := range p.addrs {
		eligible := true
		for _, criteria := range criteriaList {
			if !criteria.Eligible(p.leases[addr.String()], addr) {
				eligible = false
			}
		}
		if eligible {
			eligibleAddrs = append(eligibleAddrs, addr)
		}
	}
	log.Infof("Found %d eligible addresses meeting criteria: %v", len(eligibleAddrs), eligibleAddrs)
	// Pick one of the eligible addresses at random
	addr := eligibleAddrs[rand.Intn(len(eligibleAddrs))]
	// Lease it
	log.Infof("Leasing %s", addr)
	p.leases[addr.String()] = append(p.leases[addr.String()], ctx)
	// When the context cancels release the lease
	go func() {
		<-ctx.Done()
		log.Infof("Releasing lease on %s", addr)
		for idx, lctx := range p.leases[addr.String()] {
			if lctx == ctx {
				p.leases[addr.String()] = append(p.leases[addr.String()][:idx], p.leases[addr.String()][idx+1:]...)
				break
			}
		}
	}()
	// Return the leased address
	return addr
}
