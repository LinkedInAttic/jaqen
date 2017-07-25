/*
 * Copyright 2017 LinkedIn Corporation. All rights reserved. Licensed under the BSD-2 Clause license.
 * See LICENSE in the project root for license information.
 */

package main

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"sync"
	"time"

	//	"github.com/satori/go.uuid"

	"github.com/tylerb/graceful"
)

// HTTPServer represents a server that can handle HTTP requests
type HTTPServer struct {
	Address *Address         // Mainly used for debugging
	Server  *graceful.Server // http.Server has no clean way to shutdown, use graceful as a substitute
	WG      *sync.WaitGroup  // We use a WaitGroup to cleanup the server when no active rebinds reference it anymore
}

// Used for matching UUIDs (ending in a dot for subdomains)
var SubdomainRegex = regexp.MustCompile("^([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\\.")

// CreateHTTPServer creates a *HTTPServer instance and adds it to the HTTPServers map
func (m *RebindManager) CreateHTTPServer(ctx context.Context, addr *Address) *HTTPServer {
	// Create a graceful server because Golang needs to fix the stdlib
	var srv HTTPServer
	srv.Address = addr
	srv.WG = new(sync.WaitGroup)
	srv.WG.Add(1)
	srv.Server = &graceful.Server{
		Timeout: 5 * time.Second, // This is only used during cleanup on SIGSTOP
		Server: &http.Server{
			Addr: addr.InternalAddr(),
			Handler: http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				// If we can find a matching rebind, call the middleware
				/*matches := SubdomainRegex.FindStringSubmatch(req.Host)
				if (len(matches) > 1) {
					u, err := uuid.FromString(matches[1])
					if err == nil {
						m.RebindsLock.RLock()
						rebind, ok := m.Rebinds[u]
						m.RebindsLock.RUnlock()
						if ok {
							rebind.HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
								m.HTTPMux.ServeHTTP(rw, req.WithContext(ctx))
							}))
							return
						}
					}
				}*/
				// Inject the context into each request
				m.HTTPMux.ServeHTTP(rw, req.WithContext(ctx))
			}),
		},
	}
	// VERY VERY VERY IMPORTANT DO NOT REMOVE
	// The server must release the connection after each request so a new DNS request is triggered for each new HTTP request (assuming the cached DNS request has expired)
	// This is essential for DNS rebinding to work.
	srv.Server.Server.SetKeepAlivesEnabled(false)
	go func() {
		srv.WG.Wait()
		m.HTTPServersLock.Lock()
		log.Infof("HTTPServer: [end] - %s", addr)
		srv.Server.Stop(1 * time.Second)     // We can be aggressive here since it shouldn't be still referenced by anybody
		<-srv.Server.StopChan()              // Wait for it to actually close
		delete(m.HTTPServers, addr.String()) // Remove from list of usable servers
		m.HTTPServersLock.Unlock()
	}()
	// TODO: Are we introducing a race-condition? ("just" DoS if true)
	m.HTTPServersLock.Lock()
	m.HTTPServers[addr.String()] = &srv
	m.HTTPServersLock.Unlock()
	// Begin listening in the background
	go func() {
		if err := srv.Server.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()
	return &srv
}

// IndexHandler handles requests for the index page
func (m *RebindManager) IndexHandler(w http.ResponseWriter, req *http.Request) {
	io.WriteString(w, "Index")
}

// PingHandler handles requests for the ping page
func (m *RebindManager) PingHandler(w http.ResponseWriter, req *http.Request) {
	io.WriteString(w, "pong")
}

// JSHandler handles requests for the js
func (m *RebindManager) JSHandler(w http.ResponseWriter, req *http.Request) {
	http.ServeFile(w, req, "www/rebind.js")
}

// RebindHandler handles requests for a given host
func (m *RebindManager) RebindHandler(w http.ResponseWriter, req *http.Request) {
	// Let the rebind method handle the request
	http.ServeFile(w, req, "www/frame.html")
}

// CacheHandler handles requests for a given host
func (m *RebindManager) CacheHandler(w http.ResponseWriter, req *http.Request) {
	// Let the rebind method handle the request
	http.ServeFile(w, req, "www/frame.appcache")
}
