package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/miekg/dns"
)

var (
	zones *string
	qtype *string
	num   *int
)

type Result struct {
	Length int
	Error  bool
	Time   int64
}

func sendQuery(name string, ch chan Result) {
	var length int

	c := new(dns.Client)
	c.DialTimeout = 15 * time.Second
	c.ReadTimeout = 15 * time.Second
	c.WriteTimeout = 15 * time.Second
	question := new(dns.Msg)
	question.SetQuestion(name, dns.TypeSOA)

	start := time.Now()
	msg, _, err := c.Exchange(question, "127.0.0.1:5354")
	elapsed := int64(time.Since(start))
	if err != nil {
		log.Println(fmt.Sprintf("ERROR: Querying SOA for %s : %s", name, err))
		length = 0
	} else {
		length = len(msg.Answer)
	}

	result := Result{Length: length, Error: err != nil, Time: elapsed}
	ch <- result
}

func doAxfr(name string, ch chan Result) {
	length := 0

	t := new(dns.Transfer)
	t.DialTimeout = 5 * time.Second
	t.ReadTimeout = 5 * time.Second
	t.WriteTimeout = 5 * time.Second
	m := new(dns.Msg)
	m.SetAxfr(name)
	rrs := []dns.RR{}

	start := time.Now()
	transch, err := t.In(m, "127.0.0.1:5354")
	for env := range transch {
		rrs = append(rrs, env.RR...)
	}
	elapsed := int64(time.Since(start))
	if err != nil {
		log.Println(fmt.Sprintf("ERROR: AXFR for %s : %s", name, err))
	} else {
		length = len(rrs)
	}
	result := Result{Length: length, Error: err != nil, Time: elapsed}
	ch <- result
}

func main() {
	zones = flag.String("zones", "", "CSV list of zone names")
	qtype = flag.String("qtype", "SOA", "Type of query to send {SOA, AXFR}")
	num = flag.Int("queries", 10, "The number of queries to send for each zone")

	flag.Parse()

	log.Println(fmt.Sprintf("Finna %s %s %d times", *qtype, *zones, *num))

	ch := make(chan Result)
	results := []Result{}

	start := time.Now()
	for i := 0; i < *num; i++ {
		if *qtype == "SOA" {
			go sendQuery(*zones, ch)
		} else if *qtype == "AXFR" {
			go doAxfr(*zones, ch)
		}
	}

	for i := 0; i < *num; i++ {
		rez := <-ch
		results = append(results, rez)
	}
	elapsed := time.Since(start)

	totalTime := int64(elapsed / time.Millisecond)

	numErrors := 0
	totalLen := 0
	cumTime := int64(0)
	for _, r := range results {
		if r.Error == true {
			numErrors += 1
		}
		totalLen += r.Length
		cumTime += r.Time / int64(time.Millisecond)
	}
	avgLen := float64(totalLen) / float64(*num)
	avgTime := float64(cumTime) / float64(*num)

	log.Println(fmt.Sprintf("Total execution time was %d ms", totalTime))
	log.Println(fmt.Sprintf("Queries per second: %f", (float64(*num)/float64(totalTime))*1000))
	log.Println(fmt.Sprintf("Total Errors: %d", numErrors))
	log.Println(fmt.Sprintf("Avg Response Duration: %f ms", avgTime))
	log.Println(fmt.Sprintf("Avg Response Length: %f rrs", avgLen))
}
