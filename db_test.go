package mdns_test

import (
	"database/sql"
	"fmt"
	"github.com/miekg/dns"
	"testing"

	"github.com/rackerlabs/mdns"
)

func TestMySQLOpen(t *testing.T) {
	SetUp()

	mysql := &mdns.MySQLDriver{}
	ok(t, mysql.Open())
}

func TestMySQLOpenInvalidDB(t *testing.T) {
	SetUp()

	mdns.Conf.DbConn = ".1:3307)/designate"
	mysql := &mdns.MySQLDriver{}
	err := mysql.Open()
	assert(t, err != nil, "There should have been in error connecting to .1:3307)/designate")
}

func TestDBGetAxfr(t *testing.T) {
	SetUp()

	mysql := &mdns.MySQLDriver{}
	ok(t, mysql.Open())

	storage := mdns.Storage{Driver: mysql}

	rrs, err := storage.Driver.GetFullAxfrRRs("gomdns.com.")
	assert(t, err == nil, fmt.Sprintf("There was an error getting axfr rrs: %s", err))
	assert(t, len(rrs) == 3, fmt.Sprintf("Wrong number of records: %d", len(rrs)))
}

func TestDBGetAxfrBadDB(t *testing.T) {
	SetUp()

	// Connect to the right database
	mdns.Conf.DbConn = "root:password@tcp(127.0.0.1:3306)/mysql"
	mysql := &mdns.MySQLDriver{}
	ok(t, mysql.Open())

	storage := mdns.Storage{Driver: mysql}

	_, err := storage.Driver.GetFullAxfrRRs("gomdns.com.")
	assert(t, err != nil, "There should have been an error")
}

func TestDBSOAQuery(t *testing.T) {
	SetUp()

	mysql := &mdns.MySQLDriver{}
	ok(t, mysql.Open())

	storage := mdns.Storage{Driver: mysql}

	rrs, err := storage.Driver.GetQueryRRs("gomdns.com.", "SOA")
	assert(t, err == nil, fmt.Sprintf("There was an error getting axfr rrs: %s", err))
	assert(t, len(rrs) == 1, fmt.Sprintf("Wrong number of records: %d", len(rrs)))
	serial := rrs[0].(*dns.SOA).Serial
	assert(t, serial == 1458672783,
		fmt.Sprintf("Wrong serial number, expected 1458672783, got: %d", serial))
}

func TestDBSOAQueryBadDB(t *testing.T) {
	SetUp()

	// Connect to the right database
	mdns.Conf.DbConn = "root:password@tcp(127.0.0.1:3306)/mysql"
	mysql := &mdns.MySQLDriver{}
	ok(t, mysql.Open())

	storage := mdns.Storage{Driver: mysql}

	_, err := storage.Driver.GetQueryRRs("gomdns.com.", "SOA")
	assert(t, err != nil, "There should have been an error")
}

func TestBuildDNSRRsNoSOA(t *testing.T) {
	SetUp()

	zone := mdns.Zone{Id: "foo.com.", Ttl: 300}
	rrs := []mdns.RR{
		mdns.RR{Id: "1", Rrtype: "A", Ttl: sql.NullInt64{Int64: 300, Valid: true}, Name: "bar.foo.com.", Data: "158.85.167.186", Action: "UPDATE", Created_at: "1"},
	}

	_, err := mdns.BuildDnsRRs(rrs, zone, true)
	assert(t, err != nil, "There was no error!")
}

func TestBuildDNSRRsNoRRs(t *testing.T) {
	SetUp()

	zone := mdns.Zone{Id: "foo.com.", Ttl: 300}
	rrs := []mdns.RR{}

	dnsRRs, err := mdns.BuildDnsRRs(rrs, zone, true)
	assert(t, dnsRRs == nil, fmt.Sprintf("DnsRRs wasn't []: %v", dnsRRs))
	assert(t, err != nil, "There was no error!")
}
