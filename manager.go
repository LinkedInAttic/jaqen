/*
 * Copyright 2017 LinkedIn Corporation. All rights reserved. Licensed under the BSD-2 Clause license.
 * See LICENSE in the project root for license information.
 */

package main

import (
	"context"
	"net/http"
	"sync"

	"github.com/satori/go.uuid"

	"github.com/miekg/dns"
)

// RebindManager is the "global" rebinding manager (there can be multiple instances technically)
type RebindManager struct {
	base            string
	pool            *Pool                      // Pool of IPs to use for HTTP servers
	Rebinds         map[uuid.UUID]RebindMethod // Mapping of rebinding requests to Rebinding methods
	RebindsLock     *sync.RWMutex              // Maps aren't write thread-safe (sadly)
	HTTPMux         *http.ServeMux             // Use a shared HTTP mux
	HTTPServers     map[string]*HTTPServer     // Mapping of addresses to server.
	HTTPServersLock *sync.RWMutex              // Maps aren't write thread-safe (sadly)
	DNSServers      []*dns.Server              // List of all DNS servers assoicated with the rebind manager
}

// NewRebindManager creates a *RebindManager instance
func NewRebindManager(base string, poolIPs []*Address) *RebindManager {
	m := RebindManager{
		base:            base,
		pool:            NewPool(poolIPs),
		Rebinds:         make(map[uuid.UUID]RebindMethod),
		RebindsLock:     new(sync.RWMutex),
		HTTPServers:     make(map[string]*HTTPServer),
		HTTPServersLock: new(sync.RWMutex),
	}
	m.HTTPMux = http.NewServeMux()
	m.HTTPMux.HandleFunc("/", m.IndexHandler)
	m.HTTPMux.HandleFunc("/v1.js", m.JSHandler)
	m.HTTPMux.HandleFunc("/v1.websocket", m.WebSocketHandler)
	m.HTTPMux.HandleFunc("/.well-known/rebind/v1.ping", m.PingHandler)
	m.HTTPMux.HandleFunc("/.well-known/rebind/v1.frame", m.RebindHandler)
	m.HTTPMux.HandleFunc("/.well-known/rebind/v1.appcache", m.CacheHandler)
	return &m
}

// Begin listening
func (m *RebindManager) Listen(ctx context.Context, dnsBind string, httpBinds []*Address) (wg *sync.WaitGroup, err error) {
	// Setup a DNS server for both TCP and UDP
	m.DNSServers = []*dns.Server{
		&dns.Server{
			Addr:    dnsBind,
			Handler: m,
			Net:     "tcp",
		},
		&dns.Server{
			Addr:    dnsBind,
			Handler: m,
			Net:     "udp",
		},
	}
	wg = new(sync.WaitGroup)
	// Start each of the DNS servers
	for _, srv := range m.DNSServers {
		// Start the server
		wg.Add(1)
		go func(srv *dns.Server) {
			defer wg.Done() // When this function ends, release our waitgroup
			log.Infof(`Created new DNSServer bound to "%s" (%s)`, srv.Addr, srv.Net)
			if err := srv.ListenAndServe(); err != nil {
				// Skip errors that occur during cancellation/shutdown
				// TODO: ListenAndServe doesn't gracefully eat errors when shutting down, should fix upstream
				if ctx.Err() != context.Canceled {
					log.Fatal(err)
				}
			}
			log.Infof(`Closed DNSServer bound to "%s" (%s)`, srv.Addr, srv.Net)
		}(srv)
		// Shutdown when the context is cancelled
		// TODO: When upstream dns lib supports context replace this
		go func(srv *dns.Server) {
			<-ctx.Done() // Wait until cancel
			if err := srv.Shutdown(); err != nil {
				log.Fatal(err)
			}
		}(srv)
	}
	// Lease each of the HTTP servers provided in the bind arguments
	for _, addr := range httpBinds {
		m.GetHTTPServer(ctx, m.pool.Lease(ctx, &PoolCriteriaExternalIPMatch{Addr: addr}), addr)
	}
	return
}

// GetHTTPServer will attempt to bind a http server instance to the provided bind IP on the port from addr, spawning a new one if needed
func (m *RebindManager) GetHTTPServer(ctx context.Context, bind *Address, addr *Address) (srv *HTTPServer) {
	// Build the bind address by combining the bind IP and the port from the target
	bindAddr := bind.Clone()
	bindAddr.Port = addr.Port
	// Check if we have a srv for that bind address open already
	m.HTTPServersLock.RLock()
	for _, srvInstance := range m.HTTPServers {
		if srvInstance.Address.Equal(bindAddr) {
			srv = srvInstance
		}
	}
	m.HTTPServersLock.RUnlock()
	// There is a server already, let's re-use it
	if srv != nil {
		// Increment the usage since we're checking it out
		srv.WG.Add(1)
		log.Infof(`Incremented users of HTTPServer bound to "%s" as a result of request "%s" on socket "%s"`, srv.Address, requestID(ctx), socketID(ctx))
		// We need to spawn a new server
	} else {
		srv = m.CreateHTTPServer(ctx, bindAddr)
		log.Infof(`Created HTTPServer bound to "%s" as a result of request "%s" on socket "%s"`, bindAddr, requestID(ctx), socketID(ctx))
	}
	// When the context cancels, decrement our usage of it
	go func() {
		<-ctx.Done()
		srv.WG.Done()
		log.Infof(`Decremented users of HTTPServer bound to "%s" as a result of request "%s" on socket "%s"`, srv.Address, requestID(ctx), socketID(ctx))
	}()
	return
}
