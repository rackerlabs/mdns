package main

import (
	"database/sql"
	"flag"
	"fmt"
	log "github.com/Sirupsen/logrus"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/miekg/dns"
	"github.com/vharitonsky/iniflags"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
)

var builddate = ""
var gitref = ""

// Config
var debug *bool
var bind_address *string
var bind_port *string
var db_conn *string

var db *sqlx.DB

type Zone struct {
	Id  string
	Ttl int64
}

type RR struct {
	Id     string
	Rrtype string `db:"type"`
	Ttl    sql.NullInt64
	Name   string
	Data   string
	Action string
}

//
// DNS Handling
//

func Handle(writer dns.ResponseWriter, request *dns.Msg) {
	switch request.Opcode {
	case dns.OpcodeQuery:
		if request.Question[0].Qtype == dns.TypeAXFR {
			handle_axfr(request, writer)
		} else if request.Question[0].Qtype == dns.TypeIXFR {
			writer.WriteMsg(handle_error(prep_reply(request), writer, "REFUSED"))
		} else {
			message := prep_reply(request)
			message = handle_query(request.Question[0], message, writer)
			writer.WriteMsg(message)
		}

	default:
		log.Info(fmt.Sprintf("ERROR %s : unsupported opcode %d", request.Question[0].Name, request.Opcode))
		writer.WriteMsg(handle_error(prep_reply(request), writer, "REFUSED"))
	}
}

func prep_reply(request *dns.Msg) *dns.Msg {
	question := request.Question[0]

	message := new(dns.Msg)
	message.SetReply(request)
	message.SetRcode(message, dns.RcodeSuccess)

	log.Debug(debug_request(*request, question))

	// Add the question back
	message.Question[0] = question

	// Send an authoritative answer
	message.MsgHdr.Authoritative = true

	return message
}

func handle_axfr(request *dns.Msg, writer dns.ResponseWriter) *dns.Msg {
	zonename := request.Question[0].Name
	log.Debug(fmt.Sprintf("Attempting AXFR for %s", zonename))

	rrs, err := get_axfr_rrs(zonename)
	if err != nil {
		log.Error(fmt.Sprintf("Error with the axfr for %s", zonename))
		message := prep_reply(request)
		return handle_error(message, writer, "SERVAIL")
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

func handle_query(question dns.Question, message *dns.Msg, writer dns.ResponseWriter) *dns.Msg {
	name := question.Name
	rrtypeint := question.Qtype

	// catch a panic here
	rrtype := dns.TypeToString[rrtypeint]

	log.Debug(fmt.Sprintf("Attempting %s query for %s", rrtype, name))
	rrs, err := get_rrs(name, rrtype)
	if err != nil {
		log.Error(fmt.Sprintf("There was a problem querying %s for %s", rrtype, name))
		return handle_error(message, writer, "SERVAIL")
	}

	log.Info(fmt.Sprintf("Completed %s query for %s", rrtype, name))
	if len(rrs) == 0 {
		return handle_error(message, writer, "REFUSED")
	}

	message.Answer = append(message.Answer, rrs...)
	return message
}

func handle_error(message *dns.Msg, writer dns.ResponseWriter, op string) *dns.Msg {
	question := message.Question[0]

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

//
// Database Functions
//

func init_db() error {
	var err error
	db, err = sqlx.Open("mysql", *db_conn)
	if err != nil {
		log.Error(fmt.Sprintf("Problem connecting to Database: %s", err))
		return err
	}
	// Don't defer db.Close() because we're using the db obj
	err = db.Ping()
	if err != nil {
		log.Error("Unsuccesful Ping to DB")
		return err
	}
	log.Info("Connected to the DB!")

	return nil
}

func get_axfr_rrs(zonename string) ([]dns.RR, error) {
	zone, err := get_zone(zonename)
	if err != nil {
		return nil, err
	}
	rrs, err := get_rrs_axfr(zone)
	if err != nil {
		return nil, err
	}
	return rrs, nil
}

func get_zone(zonename string) (Zone, error) {
	zone := Zone{}
	row := db.QueryRowx(
		`SELECT zones.id, zones.ttl
	       FROM zones
	       WHERE zones.name = ?
	       AND zones.pool_id = '794ccc2cd75144feb57f8894c9f5c842'
	       AND zones.deleted = '0'`, zonename)
	err := row.StructScan(&zone)
	if err != nil {
		log.Error("Problem fetching zone from database : ", err)
		return zone, err
	}

	return zone, err
}

func get_rrs_axfr(zone Zone) ([]dns.RR, error) {
	var rrs []RR
	query := `SELECT recordsets.id, recordsets.type, recordsets.ttl, recordsets.name, records.data, records.action
	       FROM records
	       INNER JOIN recordsets ON records.recordset_id = recordsets.id
	       WHERE records.action != 'DELETE'
	       AND recordsets.zone_id = ?
	       ORDER BY recordsets.id`

	rows, err := db.Queryx(query, zone.Id)
	if err != nil {
		log.Error("Error fetching records: ", err)
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		rr := RR{}
		err := rows.StructScan(&rr)
		if err != nil {
			log.Error("Error parsing rr rows: ", err)
			return nil, err
		}
		rrs = append(rrs, rr)
	}
	err = rows.Err()
	if err != nil {
		log.Error("Error with rr rows: ", err)
		return nil, err
	}

	dnsrrs, err := build_dns_rrs(rrs, zone, true)
	if err != nil {
		log.Error("Error creating DNS RRs: ", err)
		return dnsrrs, err
	}
	return dnsrrs, err
}

func get_rrs(rrname string, rrtype string) ([]dns.RR, error) {
	var rrs []RR
	query := []string{`SELECT recordsets.id, recordsets.type, recordsets.ttl, recordsets.name, records.data, records.action
	       FROM records
	       INNER JOIN recordsets ON records.recordset_id = recordsets.id
	       WHERE records.action != 'DELETE'
	       AND recordsets.name = ?`}

	if rrtype != "ANY" {
		query = append(query, fmt.Sprintf("\n\t\tAND recordsets.type = '%s'", rrtype))
	}

	queryx := strings.Join(query, "")
	rows, err := db.Queryx(queryx, rrname)
	if err != nil {
		log.Error("Error querying rrs: ", err)
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		rr := RR{}
		err := rows.StructScan(&rr)
		if err != nil {
			log.Error("Error parsing rrs: ", err)
			return nil, err
		}
		rrs = append(rrs, rr)
	}
	err = rows.Err()
	if err != nil {
		log.Error("error with rr rows: ", err)
		return nil, err
	}

	// TODO: Go get the actual zone TTL
	zone := Zone{Id: "notarealzone", Ttl: 3600}
	dnsrrs, err := build_dns_rrs(rrs, zone, false)
	if err != nil {
		log.Error("Error creating DNS RRs: ", err)
		return dnsrrs, err
	}

	return dnsrrs, err
}

func build_dns_rrs(rrs []RR, zone Zone, axfr bool) ([]dns.RR, error) {
	// This could be suck inside the loop iterating the
	// DB rows, but this is much nicer. Even if it is a bit slower.
	var dnsrrs []dns.RR
	var soarecord dns.RR

	for _, rr := range rrs {
		var ttl int64
		if rr.Ttl.Valid {
			ttl = rr.Ttl.Int64
		} else {
			ttl = zone.Ttl
		}

		record := fmt.Sprintf("%s %d IN %s %s", rr.Name, ttl, rr.Rrtype, rr.Data)
		dnsrr, err := dns.NewRR(record)
		if err != nil {
			log.Error(fmt.Sprintf("Error parsing record %s: %s", record, err))
			return dnsrrs, err
		}

		log.Debug(fmt.Sprintf("Processed record %s", record))
		if rr.Rrtype != "SOA" || axfr == false {
			dnsrrs = append(dnsrrs, dnsrr)
		} else {
			soarecord = dnsrr
		}
	}

	// Put the SOA record on first and last
	if axfr == true {
		dnsrrs = append(dnsrrs, soarecord)
		dnsrrs = append([]dns.RR{soarecord}, dnsrrs...)
	}
	return dnsrrs, nil
}

//
// Utilities
//

func serve(net, ip, port string) {
	bind := fmt.Sprintf("%s:%s", ip, port)
	server := &dns.Server{Addr: bind, Net: net}

	dns.HandleFunc(".", Handle)
	log.Info(fmt.Sprintf("mdns starting %s listener on %s", net, bind))

	err := server.ListenAndServe()
	if err != nil {
		panic(fmt.Sprintf("Failed to set up the "+net+"server %s", err.Error()))
	}
}

func listen() {
	siq_quit := make(chan os.Signal)
	signal.Notify(siq_quit, syscall.SIGINT, syscall.SIGTERM)
	sig_stat := make(chan os.Signal)
	signal.Notify(sig_stat, syscall.SIGUSR1)

forever:
	for {
		select {
		case s := <-siq_quit:
			log.Info(fmt.Sprintf("Signal (%d) received, stopping", s))
			break forever
		case _ = <-sig_stat:
			log.Info(fmt.Sprintf("Goroutines: %d", runtime.NumGoroutine()))
		}
	}
}

func main() {
	// Provide a '--version' flag
	version := flag.Bool("version", false, "prints version information")
	debug = flag.Bool("debug", false, "enables debug mode")
	bind_address = flag.String("bind_address", "127.0.0.1", "IP to listen on")
	bind_port = flag.String("bind_port", "5354", "port to listen on")
	db_conn = flag.String("db", "root:password@tcp(127.0.0.1:3306)/designate", "db connection string")
	flag.Usage = func() {
		flag.PrintDefaults()
	}
	// You can specify an .ini file with the -config
	iniflags.Parse()

	// Exit if someone just wants to know version
	if *version == true {
		fmt.Println(fmt.Sprintf("built from %s on %s", gitref, builddate))
		os.Exit(0)
	}

	// Logging
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	if *debug == true {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	// Database
	err := init_db()
	if err != nil {
		log.Fatal(fmt.Sprintf("Couldn't connect to database : %s", err))
		os.Exit(1)
	}

	// Listeners
	go serve("tcp", *bind_address, *bind_port)
	go serve("udp", *bind_address, *bind_port)
	listen()
}
