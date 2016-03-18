package mdns_test

import (
	"fmt"
	"github.com/miekg/dns"
	"net"
	"testing"

	"github.com/rackerlabs/mdns"
)

//{{64796 false 0 false false false false false true false 0} false [{gomdns.com. 252 1}] [] [] [
//;; OPT PSEUDOSECTION:
//; EDNS: version 0; flags: ; udp: 4096]}

// A 'fake' dns.ResponseWriter https://github.com/miekg/dns/blob/master/server.go#L24
// is needed to pass into our DNS Handler function.
type FakeResponseWriter struct {
	writtenMsgs []dns.Msg
}

func (writer *FakeResponseWriter) LocalAddr() net.Addr {
	ret, _ := net.ResolveIPAddr("tcp", "127.0.0.1")
	return ret
}

func (writer *FakeResponseWriter) RemoteAddr() net.Addr {
	ret, _ := net.ResolveIPAddr("tcp", "127.0.0.1")
	return ret
}

func (writer *FakeResponseWriter) WriteMsg(message *dns.Msg) error {
	writer.writtenMsgs = append(writer.writtenMsgs, *message)
	return nil
}

func (writer *FakeResponseWriter) GetMsgs() []dns.Msg { return writer.writtenMsgs }

func (writer *FakeResponseWriter) Write(stuff []byte) (int, error) { return 0, nil }

func (writer *FakeResponseWriter) Close() error { return nil }

func (writer *FakeResponseWriter) TsigStatus() error { return nil }

func (writer *FakeResponseWriter) TsigTimersOnly(boo bool) {}

func (writer *FakeResponseWriter) Hijack() {}

func generateMsg(name string, qtype uint16, opcode int) dns.Msg {
	return dns.Msg{
		MsgHdr: dns.MsgHdr{
			Id:                 64796,
			Response:           false,
			Opcode:             opcode,
			Authoritative:      false,
			Truncated:          false,
			RecursionDesired:   false,
			RecursionAvailable: false,
			Zero:               false,
			AuthenticatedData:  true,
			CheckingDisabled:   false,
			Rcode:              0,
		},
		Compress: false,
		Question: []dns.Question{
			dns.Question{Name: name, Qtype: qtype, Qclass: dns.ClassINET},
		},
		Answer: []dns.RR{},
		Ns:     []dns.RR{},
		Extra:  []dns.RR{},
	}
}

func TestHandleInvalidOpcode(t *testing.T) {
	SetUp()

	mysql := &mdns.MySQLDriver{}
	ok(t, mysql.Open())

	storage := mdns.Storage{Driver: mysql}
	handler := mdns.NewDefaultMdnsHandler(storage)
	fakeWriter := &FakeResponseWriter{}
	// Send a message that mdns won't handle
	msg := generateMsg("gomdns.com.", dns.TypeNone, dns.OpcodeUpdate)

	handler.ServeDNS(fakeWriter, &msg)
	results := fakeWriter.GetMsgs()
	answer := results[0]
	assert(t, answer.Rcode == dns.RcodeRefused, fmt.Sprintf("Rcode should be 5, it was: %d", answer.Rcode))
}

func TestHandleSoaQuery(t *testing.T) {
	SetUp()

	mysql := &mdns.MySQLDriver{}
	ok(t, mysql.Open())

	storage := mdns.Storage{Driver: mysql}
	handler := mdns.NewDefaultMdnsHandler(storage)
	fakeWriter := &FakeResponseWriter{}
	// Send a message that mdns won't handle
	msg := generateMsg("gomdns.com.", dns.TypeSOA, dns.OpcodeQuery)

	handler.ServeDNS(fakeWriter, &msg)
	results := fakeWriter.GetMsgs()
	answer := results[0]
	assert(t, answer.Rcode == dns.RcodeSuccess, fmt.Sprintf("Rcode should be 0, it was: %d", answer.Rcode))
	assert(t, len(answer.Answer) == 1, fmt.Sprintf("Answer was >1 record: %d", len(answer.Answer)))
	serial := answer.Answer[0].(*dns.SOA).Serial
	assert(t, serial == 1458672783,
		fmt.Sprintf("Wrong serial number, expected 1458672783, got: %d", serial))
}

func TestHandleAxfr(t *testing.T) {
	SetUp()

	mysql := &mdns.MySQLDriver{}
	ok(t, mysql.Open())

	storage := mdns.Storage{Driver: mysql}
	handler := mdns.NewDefaultMdnsHandler(storage)
	fakeWriter := &FakeResponseWriter{}
	// Send a message that mdns won't handle
	msg := generateMsg("gomdns.com.", dns.TypeAXFR, dns.OpcodeQuery)

	handler.ServeDNS(fakeWriter, &msg)
	results := fakeWriter.GetMsgs()
	answer := results[0]
	assert(t, answer.Rcode == dns.RcodeSuccess, fmt.Sprintf("Rcode should be 0, it was: %d", answer.Rcode))
	assert(t, len(answer.Answer) == 3, fmt.Sprintf("Answer length != 3 records: %d", len(answer.Answer)))
	serial := answer.Answer[0].(*dns.SOA).Serial
	assert(t, serial == 1458672783,
		fmt.Sprintf("Wrong serial number, expected 1458672783, got: %d", serial))
	assert(t, answer.Answer[1].String() == "gomdns.com.\t3600\tIN\tNS\tns1.designate.com.",
		fmt.Sprintf("bad NS record, expecting gomdns.com.\t3600\tIN\tNS\tns1.designate.com. got :%s",
			answer.Answer[1].String()))
	serial = answer.Answer[2].(*dns.SOA).Serial
	assert(t, serial == 1458672783,
		fmt.Sprintf("Wrong serial number, expected 1458672783, got: %d", serial))
}
