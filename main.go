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
	question := request.Question[0]

	message := new(dns.Msg)
	message.SetReply(request)
	message.SetRcode(message, dns.RcodeSuccess)

	log.Debug(debug_request(*request, question, writer))

	switch request.Opcode {
	case dns.OpcodeQuery:
		if question.Qtype == dns.TypeAXFR {
			message = handle_axfr(question, message, writer)
		} else {
			message = handle_query(question, message, writer)
		}

	default:
		log.Info(fmt.Sprintf("ERROR %s : unsupported opcode %d", question.Name, request.Opcode))
		message = handle_error(message, writer, "REFUSED")
	}

	respond(message, question, *request, writer)
}

func respond(message *dns.Msg, question dns.Question, request dns.Msg, writer dns.ResponseWriter) {
	// Apparently this dns library takes the question out on
	// certain RCodes, like REFUSED, which is not right. So we reinsert it.
	message.Question[0].Name = question.Name
	message.Question[0].Qtype = question.Qtype
	message.Question[0].Qclass = question.Qclass
	message.MsgHdr.Opcode = request.Opcode

	// Send an authoritative answer
	message.MsgHdr.Authoritative = true

	writer.WriteMsg(message)
}

func handle_axfr(question dns.Question, message *dns.Msg, writer dns.ResponseWriter) *dns.Msg {
	zonename := question.Name
	log.Debug(fmt.Sprintf("Attempting AXFR for %s", zonename))
	rrs, err := do_axfr(zonename)
	if err != nil {
		log.Error(fmt.Sprintf("Error with the axfr for %s", zonename))
		return handle_error(message, writer, "SERVAIL")
	}

	message.Answer = append(message.Answer, rrs...)
	log.Info(fmt.Sprintf("Completed AXFR for %s", zonename))
	return message
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
	switch op {
	case "REFUSED":
		message.SetRcode(message, dns.RcodeRefused)
	case "SERVFAIL":
		message.SetRcode(message, dns.RcodeServerFailure)
	default:
		message.SetRcode(message, dns.RcodeServerFailure)
	}

	return message
}

func debug_request(request dns.Msg, question dns.Question, writer dns.ResponseWriter) string {
	addr := writer.RemoteAddr().String() // ipaddr string
	s := []string{}
	s = append(s, fmt.Sprintf("Received request from %s ", addr))
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

func do_axfr(zonename string) ([]dns.RR, error) {
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
	bind_port = flag.String("bind_port", "5358", "port to listen on")
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
