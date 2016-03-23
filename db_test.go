package mdns_test

import (
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
