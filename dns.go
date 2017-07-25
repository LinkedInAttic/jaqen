/*
 * Copyright 2017 LinkedIn Corporation. All rights reserved. Licensed under the BSD-2 Clause license.
 * See LICENSE in the project root for license information.
 */

package main

import (
	"github.com/satori/go.uuid"

	"github.com/miekg/dns"
)

// DNSAnswer represents a answer to a DNS question
type DNSAnswer struct {
	TTL     uint32
	Address *Address
}

// ServeDefaultDNS handles unknown DNS requests, returning the default HTTP bind
func (m *RebindManager) ServeDefaultDNS(w dns.ResponseWriter, req *dns.Msg) {
	r := new(dns.Msg)
	r.SetReply(req)
	// Refuse the request
	r.Rcode = dns.RcodeRefused
	err := w.WriteMsg(r)
	if err != nil {
		log.Error(err)
	}
}

// ServeDNS handles DNS requests, either returning the matching
func (m *RebindManager) ServeDNS(w dns.ResponseWriter, req *dns.Msg) {
	log.Debugf("Got DNS Request: %s", req.Question[0].String())
	// Extract out the UUID
	matches := SubdomainRegex.FindStringSubmatch(req.Question[0].Name)
	if len(matches) != 2 {
		m.ServeDefaultDNS(w, req)
		return
	}
	u, err := uuid.FromString(matches[1])
	if err != nil {
		m.ServeDefaultDNS(w, req)
		return
	}
	// Check if we have a known rebind method for it
	m.RebindsLock.RLock()
	rebind, exists := m.Rebinds[u]
	m.RebindsLock.RUnlock()
	if !exists {
		m.ServeDefaultDNS(w, req)
		return
	}
	// Create a reply message
	r := new(dns.Msg)
	r.SetReply(req)
	// Call the "Handle" method to get our answers
	answers := rebind.HandleDNS(req.Question[0].Qtype)
	// Iterate over each answer and add to the dns message
	r.Answer = make([]dns.RR, len(answers))
	for idx, answer := range answers {
		// Switch based on the request type
		hdr := dns.RR_Header{
			Name:   req.Question[0].Name,
			Rrtype: req.Question[0].Qtype,
			Class:  dns.ClassINET,
			Ttl:    answer.TTL,
		}
		switch req.Question[0].Qtype {
		case dns.TypeA:
			r.Answer[idx] = &dns.A{
				Hdr: hdr,
				A:   answer.Address.IP(),
			}
		case dns.TypeAAAA:
			r.Answer[idx] = &dns.AAAA{
				Hdr:  hdr,
				AAAA: answer.Address.IP(),
			}
		case dns.TypeCNAME:
			r.Answer[idx] = &dns.CNAME{
				Hdr:    hdr,
				Target: answer.Address.Host,
			}
		}
	}
	//log.Debugf("Answered DNS Request: %s", r)
	// Send the actual DNS response
	err = w.WriteMsg(r)
	if err != nil {
		log.Error(err)
	}
}
