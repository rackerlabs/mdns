package mdns

import (
	"database/sql"
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
		get_axfr_rrs(string) ([]dns.RR, error)
		get_rrs_axfr(Zone) ([]dns.RR, error)
		get_rrs(string, string) ([]dns.RR, error)
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
	Id     string
	Rrtype string `db:"type"`
	Ttl    sql.NullInt64
	Name   string
	Data   string
	Action string
}

//
// MySQL Driver Functions
//

func (mysql *MySQLDriver) Open() error {
	var err error
	mysql.db, err = sqlx.Open(Conf.Db_type, Conf.Db_conn)
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

func (mysql *MySQLDriver) get_axfr_rrs(zonename string) ([]dns.RR, error) {
	zone, err := mysql.get_zone(zonename)
	if err != nil {
		return nil, err
	}
	rrs, err := mysql.get_rrs_axfr(zone)
	if err != nil {
		return nil, err
	}
	return rrs, nil
}

func (mysql *MySQLDriver) get_zone(zonename string) (Zone, error) {
	zone := Zone{}
	row := mysql.db.QueryRowx(
		`SELECT zones.id, zones.ttl
	       FROM zones
	       WHERE zones.name = ?
	       AND zones.pool_id = '794ccc2cd75144feb57f8894c9f5c842'
	       AND zones.deleted = '0'`, zonename)
	err := row.StructScan(&zone)
	if err != nil {
		return zone, err
	}

	return zone, err
}

func (mysql *MySQLDriver) get_rrs_axfr(zone Zone) ([]dns.RR, error) {
	var rrs []RR
	query := `SELECT recordsets.id, recordsets.type, recordsets.ttl, recordsets.name, records.data, records.action
	       FROM records
	       INNER JOIN recordsets ON records.recordset_id = recordsets.id
	       WHERE records.action != 'DELETE'
	       AND recordsets.zone_id = ?
	       ORDER BY recordsets.id`

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

	dnsrrs, err := build_dns_rrs(rrs, zone, true)
	if err != nil {
		log.Error("Error creating DNS RRs: ", err)
		return dnsrrs, err
	}
	return dnsrrs, err
}

func (mysql *MySQLDriver) get_rrs(rrname string, rrtype string) ([]dns.RR, error) {
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
	rows, err := mysql.db.Queryx(queryx, rrname)
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
