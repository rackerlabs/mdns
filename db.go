package mdns

import (
	"database/sql"
	"errors"
	"fmt"
	log "github.com/Sirupsen/logrus"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/miekg/dns"
	"strings"
)

//
// Types
//

type Storage struct {
	Driver interface {
		Open() error
		GetFullAxfrRRs(string) ([]dns.RR, error)
		getZone(string) (Zone, error)
		getRawAxfrRRs(Zone) ([]dns.RR, error)
		GetQueryRRs(string, string) ([]dns.RR, error)
	}
}

type MySQLDriver struct {
	db *sqlx.DB
}

type Zone struct {
	Id  string
	Ttl int64
}

type RR struct {
	Id         string
	Rrtype     string `db:"type"`
	Ttl        sql.NullInt64
	Name       string
	Data       string
	Action     string
	Created_at string
}

//
// MySQL Driver Functions
//

func (mysql *MySQLDriver) Open() error {
	var err error
	mysql.db, err = sqlx.Open(Conf.DbType, Conf.DbConn)
	if err != nil {
		log.Error(fmt.Sprintf("Problem connecting to Database: %s", err))
		return err
	}
	// Don't defer db.Close() because we're using the db obj
	err = mysql.db.Ping()
	if err != nil {
		log.Error("Unsuccesful Ping to DB")
		return err
	}
	log.Info("Connected to the DB!")

	return nil
}

func (mysql *MySQLDriver) GetFullAxfrRRs(zonename string) ([]dns.RR, error) {
	zone, err := mysql.getZone(zonename)
	if err != nil {
		return nil, err
	}
	rrs, err := mysql.getRawAxfrRRs(zone)
	if err != nil {
		return nil, err
	}
	return rrs, nil
}

func (mysql *MySQLDriver) getZone(zonename string) (Zone, error) {
	zone := Zone{}
	row := mysql.db.QueryRowx(
		`SELECT zones.id, zones.ttl
	       FROM zones
	       WHERE zones.name = ?
	       AND zones.pool_id = '794ccc2cd75144feb57f8894c9f5c842'
	       AND zones.deleted = '0'`, zonename)
	err := row.StructScan(&zone)
	if err != nil {
		log.Error(fmt.Sprintf("Error fetching zone %s: %s", zonename, err))
		return zone, err
	}

	return zone, err
}

func (mysql *MySQLDriver) getRawAxfrRRs(zone Zone) ([]dns.RR, error) {
	var rrs []RR
	query := `SELECT recordsets.id, recordsets.type, recordsets.ttl, recordsets.name, recordsets.created_at, records.data, records.action
	       FROM records
	       INNER JOIN recordsets ON records.recordset_id = recordsets.id
	       WHERE records.action != 'DELETE'
	       AND recordsets.zone_id = ?
	       ORDER BY recordsets.created_at`

	rows, err := mysql.db.Queryx(query, zone.Id)
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

	dnsRRs, err := BuildDnsRRs(rrs, zone, true)
	if err != nil {
		log.Error("Error creating DNS RRs: ", err)
		return dnsRRs, err
	}
	return dnsRRs, err
}

func (mysql *MySQLDriver) GetQueryRRs(RRName string, RRType string) ([]dns.RR, error) {
	var rrs []RR
	query := []string{`SELECT recordsets.id, recordsets.type, recordsets.ttl, recordsets.name, recordsets.created_at, records.data, records.action
	       FROM records
	       INNER JOIN recordsets ON records.recordset_id = recordsets.id
	       WHERE records.action != 'DELETE'
	       AND recordsets.name = ?`}

	if RRType != "ANY" {
		query = append(query, fmt.Sprintf("\n\t\tAND recordsets.type = '%s'", RRType))
	}

	queryx := strings.Join(query, "")
	rows, err := mysql.db.Queryx(queryx, RRName)
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
	DnsRRs, err := BuildDnsRRs(rrs, zone, false)
	if err != nil {
		log.Error("Error creating DNS RRs: ", err)
		return DnsRRs, err
	}

	return DnsRRs, err
}

func BuildDnsRRs(rrs []RR, zone Zone, axfr bool) ([]dns.RR, error) {
	// This could be suck inside the loop iterating the
	// DB rows, but this is much nicer. Even if it is a bit slower.
	var DnsRRs []dns.RR
	var SoaRecord dns.RR

	for _, rr := range rrs {
		var ttl int64
		if rr.Ttl.Valid {
			ttl = rr.Ttl.Int64
		} else {
			ttl = zone.Ttl
		}

		record := fmt.Sprintf("%s %d IN %s %s", rr.Name, ttl, rr.Rrtype, rr.Data)
		DnsRR, err := dns.NewRR(record)
		if err != nil {
			log.Error(fmt.Sprintf("Error parsing record %s: %s", record, err))
			return DnsRRs, err
		}

		log.Debug(fmt.Sprintf("Processed record %s", record))
		if rr.Rrtype != "SOA" || axfr == false {
			DnsRRs = append(DnsRRs, DnsRR)
		} else {
			SoaRecord = DnsRR
		}
	}

	// Put the SOA record on first and last
	if axfr == true {
		if SoaRecord == nil {
			return DnsRRs, errors.New("No SOA record found in AXFR")
		}
		DnsRRs = append(DnsRRs, SoaRecord)
		DnsRRs = append([]dns.RR{SoaRecord}, DnsRRs...)
	}
	return DnsRRs, nil
}
