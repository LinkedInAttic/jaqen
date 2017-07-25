/*
 * Copyright 2017 LinkedIn Corporation. All rights reserved. Licensed under the BSD-2 Clause license.
 * See LICENSE in the project root for license information.
 */

package main

import (
	"context"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"

	"github.com/jessevdk/go-flags"
)

// Setup the logger to be used globally (thread-safe)
var log = logrus.New()

// Setup the CLI options
type DNSOptions struct {
	Bind string `long:"dns-bind" description:"Address to bind the DNS listeners to" required:"true"`
}
type HTTPOptions struct {
	Bind    []string `long:"http-bind" description:"Address(es) to bind the main HTTP listener to" required:"true"`
	Pool    []string `long:"http-pool" description:"The pool of IP addresses to use for HTTP requests" required:"true"`
	BindMap []string `long:"http-bind-map" description:"A mapping of internal->external IPs to use when binding to addresses"`
}
type Options struct {
	Base    string      `short:"b" long:"base-uri" description:"The base URI to serve files from" required:"true"`
	Verbose []bool      `short:"v" long:"verbose" description:"Verbose output"`
	DNS     DNSOptions  `group:"DNS Options"`
	HTTP    HTTPOptions `group:"HTTP Options"`
}

var opts Options
var parser = flags.NewParser(&opts, flags.Default)

// Entry Point
func main() {

	// Seed the random number generator NOTE: Not a security issue, it's okay if this is predictable
	rand.Seed(time.Now().Unix())

	// Parse the CLI args
	if _, err := parser.Parse(); err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		} else {
			os.Exit(1)
		}
	}

	// Set the log level based on -v
	// TODO: Handle too many "-v"s as max verbosity?
	switch len(opts.Verbose) {
	default:
		log.Level = logrus.ErrorLevel
	case 1:
		log.Level = logrus.WarnLevel
	case 2:
		log.Level = logrus.InfoLevel
	case 3:
		log.Level = logrus.DebugLevel
	}

	// Cast the bind addresses and make sure they're valid
	binds := make([]*Address, len(opts.HTTP.Bind))
	for idx, rawIP := range opts.HTTP.Bind {
		addr := NewAddress(rawIP)
		if addr == nil {
			log.Fatalf("Couldn't parse HTTP Bind address: %s", rawIP)
		}
		binds[idx] = addr
	}

	// Cast the pool ips and make sure they're valid
	pool := make([]*Address, len(opts.HTTP.Pool))
	for idx, rawIP := range opts.HTTP.Pool {
		addr := NewAddress(rawIP)
		if addr == nil {
			log.Fatalf("Couldn't parse HTTP pool IP: %s", rawIP)
		}
		pool[idx] = addr
	}

	// If we were provided bind-map, update the pool IPs with their internal/external bindings
	for _, bindMap := range opts.HTTP.BindMap {
		rawIPs := strings.SplitN(bindMap, "/", 2)
		addrInternal := NewAddress(rawIPs[0])
		if addrInternal == nil {
			log.Fatalf("Couldn't parse HTTP bind-map internal IP: %s", addrInternal)
		}
		addrExternal := NewAddress(rawIPs[1])
		if addrExternal == nil {
			log.Fatalf("Couldn't parse HTTP bind-map external IP: %s", addrExternal)
		}
		// Loop over each addr in the pool until we find the match
		for _, addr := range pool {
			if addr.IP().Equal(addrInternal.IP()) {
				addr.ExternalIP = addrExternal.IP()
			}
		}
	}

	// Create a base context for this main thread
	ctx, triggerShutdown := context.WithCancel(context.Background())

	// Create a new rebind manager with the provided options
	mgr := NewRebindManager(opts.Base, pool)

	// Begin listening
	listenersWg, err := mgr.Listen(ctx, opts.DNS.Bind, binds)
	if err != nil {
		log.Error(err)
	}

	// Listen for a sigstop
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	for range c {
		// Cancel the main context, this should trigger a shutdown
		triggerShutdown()
		// Wait for all listening servers to cleanup gracefully before exiting
		listenersWg.Wait()
		break
	}
}
