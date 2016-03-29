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
	storage   Storage
	axfrFunc  func(dns.ResponseWriter, *dns.Msg, Storage) error
	queryFunc func(dns.Question, *dns.Msg, Storage) (*dns.Msg, error)
	errorFunc func(*dns.Msg, string) *dns.Msg
}

func NewDefaultMdnsHandler(storage Storage) MdnsHandler {
	return MdnsHandler{
		axfrFunc:  handleAXFR,
		queryFunc: handleQuery,
		errorFunc: handleError,
		storage:   storage,
	}
}

func (mdns *MdnsHandler) ServeDNS(writer dns.ResponseWriter, request *dns.Msg) {
	log.Debug(debugRequest(*request, request.Question[0]))

	var message *dns.Msg
	var err error

	switch request.Opcode {
	case dns.OpcodeQuery:
		if request.Question[0].Qtype == dns.TypeAXFR {
			err = mdns.axfrFunc(writer, request, mdns.storage)
			if err != nil {
				log.Error(fmt.Sprintf("Problem with AXFR for %s: %s", request.Question[0].Name, err))
				message = mdns.errorFunc(request, "SERVFAIL")
			} else {
				return
			}
		} else if request.Question[0].Qtype == dns.TypeIXFR {
			message = mdns.errorFunc(request, "REFUSED")
		} else {
			message = PrepReply(request)
			message, err = mdns.queryFunc(request.Question[0], message, mdns.storage)
			if err != nil {
				message = mdns.errorFunc(request, err.Error())
			}
		}

	default:
		log.Info(fmt.Sprintf("ERROR %s : unsupported opcode %d", request.Question[0].Name, request.Opcode))
		message = mdns.errorFunc(request, "REFUSED")
	}

	writer.WriteMsg(message)
}

func PrepReply(request *dns.Msg) *dns.Msg {
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

func handleError(message *dns.Msg, op string) *dns.Msg {
	question := message.Question[0]
	message = PrepReply(message)

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

func debugRequest(request dns.Msg, question dns.Question) string {
	s := []string{}
	s = append(s, fmt.Sprintf("Received request "))
	s = append(s, fmt.Sprintf("for %s ", question.Name))
	s = append(s, fmt.Sprintf("opcode: %d ", request.Opcode))
	s = append(s, fmt.Sprintf("RRType: %d ", question.Qtype))
	s = append(s, fmt.Sprintf("rrclass: %d ", question.Qclass))
	return strings.Join(s, "")
}

func handleAXFR(writer dns.ResponseWriter, request *dns.Msg, storage Storage) error {
	zonename := request.Question[0].Name
	log.Debug(fmt.Sprintf("Attempting AXFR for %s", zonename))

	rrs, err := storage.Driver.GetFullAxfrRRs(zonename)
	if err != nil {
		return err
	}

	err = sendAxfr(writer, request, rrs)
	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("Completed AXFR for %s", zonename))
	return nil
}

func sendAxfr(writer dns.ResponseWriter, request *dns.Msg, rrs []dns.RR) error {
	zonename := request.Question[0].Name
	envelopes := []dns.Envelope{}

	SentRRs := 0
	for SentRRs < len(rrs) {
		RRsToSend := 100
		if SentRRs+RRsToSend > len(rrs) {
			RRsToSend = len(rrs) - SentRRs
		}
		envelopes = append(envelopes, dns.Envelope{RR: rrs[SentRRs:(SentRRs + RRsToSend)]})
		SentRRs += RRsToSend
	}

	for _, envelope := range envelopes {
		message := PrepReply(request)
		message.Answer = append(message.Answer, envelope.RR...)
		if err := writer.WriteMsg(message); err != nil {
			log.Error(fmt.Sprintf("Error answering axfr for %s: %s", zonename, err))
			return err
		}
	}

	return nil

}

func handleQuery(question dns.Question, message *dns.Msg, storage Storage) (*dns.Msg, error) {
	name := question.Name
	RawRRType := question.Qtype

	// catch a panic here
	RRType := dns.TypeToString[RawRRType]

	log.Debug(fmt.Sprintf("Attempting %s query for %s", RRType, name))
	rrs, err := storage.Driver.GetQueryRRs(name, RRType)
	if err != nil {
		log.Error(fmt.Sprintf("There was a problem querying %s for %s", RRType, name))
		return message, errors.New("SERVFAIL")
	}

	log.Info(fmt.Sprintf("Completed %s query for %s", RRType, name))
	if len(rrs) == 0 {
		return message, errors.New("REFUSED")
	}

	message.Answer = append(message.Answer, rrs...)
	return message, nil
}
