//go:build darwin && !cgo

package local

import (
	"context"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/dns"

	mDNS "github.com/miekg/dns"
)

// systemExchange provides a cgo-free fallback for darwin builds.
// In !cgo mode we cannot call Apple's resolver APIs, so we fallback to
// the generic exchange path using parsed system resolvers.
func (t *Transport) systemExchange(ctx context.Context, message *mDNS.Msg) (*mDNS.Msg, error) {
	question := message.Question[0]
	if t.hosts != nil && (question.Qtype == mDNS.TypeA || question.Qtype == mDNS.TypeAAAA) {
		addresses := t.hosts.Lookup(dns.FqdnToDomain(question.Name))
		if len(addresses) > 0 {
			return dns.FixedResponse(message.Id, question, addresses, C.DefaultDNSTTL), nil
		}
	}
	if t.dhcpTransport != nil {
		dhcpServers := t.dhcpTransport.Fetch()
		if len(dhcpServers) > 0 {
			return t.dhcpTransport.Exchange0(ctx, message, dhcpServers)
		}
	}
	return t.exchange(ctx, message, question.Name)
}
