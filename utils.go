package mdns

import (
	"flag"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/miekg/dns"
	"github.com/vharitonsky/iniflags"
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

//
// Config
//
var Conf Config

type Config struct {
	Version      bool
	Debug        bool
	Bind_address string
	Bind_port    string
	Db_type      string
	Db_conn      string
}

func Init_config() Config {
	// Provide a '--version' flag
	version := flag.Bool("version", false, "prints version information")
	debug := flag.Bool("debug", false, "enables debug mode")
	bind_address := flag.String("bind_address", "127.0.0.1", "IP to listen on")
	bind_port := flag.String("bind_port", "5354", "port to listen on")
	db_type := flag.String("db_type", "mysql", "type of db connection (mysql, postgres, sqlite3)")
	db_conn := flag.String("db", "root:password@tcp(127.0.0.1:3306)/designate", "db connection string")
	flag.Usage = func() {
		flag.PrintDefaults()
	}
	// You can specify an .ini file with the -config
	iniflags.Parse()
	Conf = Config{
		Version:      *version,
		Debug:        *debug,
		Bind_address: *bind_address,
		Bind_port:    *bind_port,
		Db_type:      *db_type,
		Db_conn:      *db_conn,
	}
	return Conf
}

func Init_logging() {
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	if Conf.Debug == true {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}
}

//
// Utilities
//

func Serve(net, ip, port string, handler MdnsHandler) {
	bind := fmt.Sprintf("%s:%s", ip, port)
	server := &dns.Server{Addr: bind, Net: net, Handler: &handler}

	log.Info(fmt.Sprintf("mdns starting %s listener on %s", net, bind))

	err := server.ListenAndServe()
	if err != nil {
		panic(fmt.Sprintf("Failed to set up the "+net+"server %s", err.Error()))
	}
}

func Listen() {
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
