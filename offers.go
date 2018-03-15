/*
 * Copyright 2017 LinkedIn Corporation. All rights reserved. Licensed under the BSD-2 Clause license.
 * See LICENSE in the project root for license information.
 */

package main

import (
	"context"
	"fmt"
	"reflect"

	"github.com/satori/go.uuid"
)

// RebindOffer describes a offer to rebind
type RebindOffer struct {
	ID  uuid.UUID `json:"id"`
	URL string    `json:"url"`
}

// MakeOffer is responsible for setting up then "offering" multiple rebinds for a given request
func (m *RebindManager) MakeOffer(ctx context.Context, req WebSocketHostRequest) []RebindOffer {
	// TODO: Choose slightly more intelligently
	methods := []RebindMethod{
		NewTTLRebind(ctx, m, req.Host, 1),
		NewTTLRebind(ctx, m, req.Host, 2),
		NewTTLRebind(ctx, m, req.Host, 4),
		NewTTLRebind(ctx, m, req.Host, 8),
		NewTTLRebind(ctx, m, req.Host, 16),
		NewThresholdRebind(ctx, m, req.Host, 1, 2),
		NewThresholdRebind(ctx, m, req.Host, 2, 2),
		NewThresholdRebind(ctx, m, req.Host, 3, 4),
		NewThresholdRebind(ctx, m, req.Host, 4, 4),
		/*
			&ThresholdRebind{
				Target:    req.Host,
				TTL:       2,
				Threshold: 1,
			}, &ThresholdRebind{
				Target:    req.Host,
				TTL:       2,
				Threshold: 2,
			}, &ThresholdRebind{
				Target:    req.Host,
				TTL:       4,
				Threshold: 1,
			}, &ThresholdRebind{
				Target:    req.Host,
				TTL:       4,
				Threshold: 2,
			}, &TTLRebind{
				Target: req.Host,
				TTL:    1,
			}, &TTLRebind{
				Target: req.Host,
				TTL:    2,
			}, &TTLRebind{
				Target: req.Host,
				TTL:    4,
			}, &TTLRebind{
				Target: req.Host,
				TTL:    8,
			}, &TTLRebind{
				Target: req.Host,
				TTL:    16,
			}, &TTLRebind{
				Target: req.Host,
				TTL:    32,
			},*/
		/***** DISABLED: Not refusing requests correctly *******/
		/*&MultiRecordRebind{
			Target: req.Host,
			TTL:    300,
		},*/
	}
	/***** DISABLED: Not working correctly at the moment *******/
	// If there was a cached req
	/*for _, cache := range req.Cached {
		log.Debug(cache, req.Host)
		if cache.Port() == req.Host.Port() {
			methods = append(methods, &AppCacheRebind{
				Target: req.Host,
				TTL: 1,
			})
			break
		}
	}*/
	// Loop each method and configure
	var offers []RebindOffer
	for _, method := range methods {
		id, _ := uuid.NewV4()
		offers = append(offers, RebindOffer{
			ID:  id,
			URL: fmt.Sprintf("http://%s.%s:%s/.well-known/rebind/v1.frame", id, m.base, req.Host.Port),
		})
		m.RebindsLock.Lock()
		m.Rebinds[id] = method
		m.RebindsLock.Unlock()
		log.Infof(`Created rebind offer "%s" of type "%s" for request "%s"`, id, reflect.TypeOf(method), requestID(ctx))
	}
	return offers
}
