/*
 * Copyright 2017 LinkedIn Corporation. All rights reserved. Licensed under the BSD-2 Clause license.
 * See LICENSE in the project root for license information.
 */

package main

import (
	"net/http"
)

// RebindMethod describes a generic method for triggering a rebind
type RebindMethod interface {
	HandleDNS(uint16) []DNSAnswer
	HTTPMiddleware(http.Handler) http.Handler
}
