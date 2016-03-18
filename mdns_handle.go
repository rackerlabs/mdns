package mdns

import (
	"errors"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/miekg/dns"
	"strings"
)

//
// DNS Handling
//

type MdnsHandler struct {
	storage    Storage
	axfr_func  func(dns.ResponseWriter, *dns.Msg, Storage) error
	query_func func(dns.Question, dns.ResponseWriter, *dns.Msg, Storage) (*dns.Msg, error)
	error_func func(*dns.Msg, dns.ResponseWriter, string) *dns.Msg
}

func NewDefaultMdnsHandler(storage Storage) MdnsHandler {
	return MdnsHandler{
		axfr_func:  handle_axfr,
		query_func: handle_query,
		error_func: handle_error,
		storage:    storage,
	}
}

func (mdns *MdnsHandler) ServeDNS(writer dns.ResponseWriter, request *dns.Msg) {
	log.Debug(debug_request(*request, request.Question[0]))

	var message *dns.Msg
	var err error

	switch request.Opcode {
	case dns.OpcodeQuery:
		if request.Question[0].Qtype == dns.TypeAXFR {
			err = mdns.axfr_func(writer, request, mdns.storage)
			if err != nil {
				log.Error(fmt.Sprintf("Problem with AXFR for %s: %s", request.Question[0].Name, err))
				message = mdns.error_func(request, writer, "SERVFAIL")
			} else {
				return
			}
		} else if request.Question[0].Qtype == dns.TypeIXFR {
			message = mdns.error_func(request, writer, "REFUSED")
		} else {
			message = prep_reply(request)
			message, err = mdns.query_func(request.Question[0], writer, message, mdns.storage)
			if err != nil {
				message = mdns.error_func(request, writer, err.Error())
			}
		}

	default:
		log.Info(fmt.Sprintf("ERROR %s : unsupported opcode %d", request.Question[0].Name, request.Opcode))
		message = mdns.error_func(request, writer, "REFUSED")
	}

	writer.WriteMsg(message)
}

func prep_reply(request *dns.Msg) *dns.Msg {
	question := request.Question[0]

	message := new(dns.Msg)
	message.SetReply(request)
	message.SetRcode(message, dns.RcodeSuccess)

	// Add the question back
	message.Question[0] = question

	// Send an authoritative answer
	message.MsgHdr.Authoritative = true

	return message
}

func handle_error(message *dns.Msg, writer dns.ResponseWriter, op string) *dns.Msg {
	question := message.Question[0]
	message = prep_reply(message)

	switch op {
	case "REFUSED":
		message.SetRcode(message, dns.RcodeRefused)
	case "SERVFAIL":
		message.SetRcode(message, dns.RcodeServerFailure)
	default:
		message.SetRcode(message, dns.RcodeServerFailure)
	}

	// Add the question back
	message.Question[0] = question

	// Send an authoritative answer
	message.MsgHdr.Authoritative = true

	return message
}

func debug_request(request dns.Msg, question dns.Question) string {
	s := []string{}
	s = append(s, fmt.Sprintf("Received request "))
	s = append(s, fmt.Sprintf("for %s ", question.Name))
	s = append(s, fmt.Sprintf("opcode: %d ", request.Opcode))
	s = append(s, fmt.Sprintf("rrtype: %d ", question.Qtype))
	s = append(s, fmt.Sprintf("rrclass: %d ", question.Qclass))
	return strings.Join(s, "")
}

func handle_axfr(writer dns.ResponseWriter, request *dns.Msg, storage Storage) error {
	zonename := request.Question[0].Name
	log.Debug(fmt.Sprintf("Attempting AXFR for %s", zonename))

	rrs, err := storage.Driver.get_axfr_rrs(zonename)
	if err != nil {
		return err
	}

	ch := make(chan *dns.Envelope)
	go func(ch chan *dns.Envelope, request *dns.Msg) error {
		for envelope := range ch {
			message := prep_reply(request)
			message.Answer = append(message.Answer, envelope.RR...)
			if err := writer.WriteMsg(message); err != nil {
				log.Error(fmt.Sprintf("Error answering axfr: %s", err))
				return err
			}
		}
		return nil
	}(ch, request)

	rrs_sent := 0
	for rrs_sent < len(rrs) {
		rrs_to_send := 100
		if rrs_sent+rrs_to_send > len(rrs) {
			rrs_to_send = len(rrs) - rrs_sent
		}
		ch <- &dns.Envelope{RR: rrs[rrs_sent:(rrs_sent + rrs_to_send)]}
		rrs_sent += rrs_to_send
	}
	close(ch)

	log.Info(fmt.Sprintf("Completed AXFR for %s", zonename))
	return nil
}

func handle_query(question dns.Question, writer dns.ResponseWriter, message *dns.Msg, storage Storage) (*dns.Msg, error) {
	name := question.Name
	rrtypeint := question.Qtype

	// catch a panic here
	rrtype := dns.TypeToString[rrtypeint]

	log.Debug(fmt.Sprintf("Attempting %s query for %s", rrtype, name))
	rrs, err := storage.Driver.get_rrs(name, rrtype)
	if err != nil {
		log.Error(fmt.Sprintf("There was a problem querying %s for %s", rrtype, name))
		return message, errors.New("SERVFAIL")
	}

	log.Info(fmt.Sprintf("Completed %s query for %s", rrtype, name))
	if len(rrs) == 0 {
		return message, errors.New("REFUSED")
	}

	message.Answer = append(message.Answer, rrs...)
	return message, nil
}
