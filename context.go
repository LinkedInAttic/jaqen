/*
 * Copyright 2017 LinkedIn Corporation. All rights reserved. Licensed under the BSD-2 Clause license.
 * See LICENSE in the project root for license information.
 */

package main

import (
	"context"

	"github.com/satori/go.uuid"
)

// This file is a helper class for information stored within a context

// These keys represent
const (
	socketIDKey  string = "socketID"
	requestIDKey string = "requestID"
)

// socketID retrieves the socket ID from the provided context
func socketID(ctx context.Context) uuid.UUID {
	val := ctx.Value(socketIDKey)
	if val == nil {
		return uuid.UUID{}
	}
	return val.(uuid.UUID)
}

// requestID retrieves the request ID from the provided context
func requestID(ctx context.Context) uuid.UUID {
	val := ctx.Value(requestIDKey)
	if val == nil {
		return uuid.UUID{}
	}
	return val.(uuid.UUID)
}
