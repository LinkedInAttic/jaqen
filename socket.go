/*
 * Copyright 2017 LinkedIn Corporation. All rights reserved. Licensed under the BSD-2 Clause license.
 * See LICENSE in the project root for license information.
 */

package main

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/satori/go.uuid"

	"github.com/gorilla/websocket"
)

// upgrader is a global upgrader, because why not
// TODO: Come up with a better reason other than "why not" or fix it
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // This is normally bad practice, but for this tool it's intentional
	},
}

// WebSocketRequest is the
type WebSocketRequest struct {
	RequestID uuid.UUID `json:"requestId"`
	Action    string    `json:"action"`
}

// WebSocketInitRequest is the initial request
type WebSocketInitRequest struct {
	Navigator struct {
		UserAgent string `json:"userAgent"`
	} `json:"navigator"`
}

// WebSocketHostRequest is the request to offer rebinds for a given host
type WebSocketHostRequest struct {
	Host *Address `json:"host"`
}

// WebSocketHostResponse is the request to offer rebinds for a given host
type WebSocketHostResponse struct {
	RequestID uuid.UUID     `json:"requestId"`
	Offers    []RebindOffer `json:"offers"`
}

// WebSocketMessageHandler handles parsed messages from the socket, returns a list of waitgroups to decrement when the socket closes and any errors that occured
func (m *RebindManager) WebSocketMessageHandler(ctx context.Context, conn *websocket.Conn, wReq WebSocketRequest, rawMsg []byte) error {
	log.Infof(`Socket "%s" got msg "%s" for "%s" action`, socketID(ctx), requestID(ctx), wReq.Action)
	switch wReq.Action {
	case "host":
		// Parse the message
		var msg WebSocketHostRequest
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			return err
		}
		log.Debug(msg)
		// Make rebind offers based on the information provided by the host
		offers := m.MakeOffer(ctx, msg)
		// Marshal into a response and write it back
		resp := &WebSocketHostResponse{
			RequestID: wReq.RequestID,
			Offers:    offers,
		}
		rResp, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		err = conn.WriteMessage(websocket.TextMessage, rResp)
		if err != nil {
			return err
		}
		log.Infof(`Wrote (%d) offers (%s) to socket "%s" in response to msg "%s"`, len(offers), offers, socketID(ctx), requestID(ctx))
	}
	return nil
}

// WebSocketHandler handles upgrading clients to a websocket connection and passing messages to the HandleMessage method
func (m *RebindManager) WebSocketHandler(w http.ResponseWriter, req *http.Request) {
	// Upgrade to a websocket
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Error(err)
		http.Error(w, err.Error(), 400)
		return
	}
	// Each socket has an ID
	id, _ := uuid.NewV4()
	log.Infof(`New socket connection "%s"`, id)
	// Create a cancel-able child context
	ctx, triggerClose := context.WithCancel(req.Context())
	ctx = context.WithValue(ctx, socketIDKey, id)
	// When the socket closes
	defer func() {
		log.Infof(`Socket "%s" has closed, cleaning up`, id)
		triggerClose() // Cancel the context
	}()
	// Loop reading messages in a queue
	for {
		// Read the init message
		_, rawMsg, err := conn.ReadMessage()
		if err != nil {
			// Ignore 1001 (going away) "errors" as they are not errors really
			if !websocket.IsCloseError(err, 1001) {
				log.Error(err)
				http.Error(w, err.Error(), 400)
			}
			return
		}
		// Read the request
		var wReq WebSocketRequest
		if err := json.Unmarshal(rawMsg, &wReq); err != nil {
			log.Error(err)
			http.Error(w, err.Error(), 400)
			return
		}
		// Add the provided requestID to the context
		ctx := context.WithValue(ctx, requestIDKey, wReq.RequestID)
		// Handle it
		if err := m.WebSocketMessageHandler(ctx, conn, wReq, rawMsg); err != nil {
			log.Error(err)
			http.Error(w, err.Error(), 400)
			return
		}
	}
}
