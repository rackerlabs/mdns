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
	Version     bool
	Debug       bool
	BindAddress string
	BindPort    string
	DbType      string
	DbConn      string
}

func InitConfig() Config {
	// Provide a '--version' flag
	version := flag.Bool("version", false, "prints version information")
	debug := flag.Bool("debug", true, "enables debug mode")
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
		Version:     *version,
		Debug:       *debug,
		BindAddress: *bind_address,
		BindPort:    *bind_port,
		DbType:      *db_type,
		DbConn:      *db_conn,
	}
	return Conf
}

//
// Logging
//

func InitLogging() {
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

	log.Info(fmt.Sprintf("starting mdns %s listener on %s", net, bind))

	err := server.ListenAndServe()
	if err != nil {
		panic(fmt.Sprintf("Failed to set up the %s server: %s", net, err.Error()))
	}
}

func Listen() {
	SigQuit := make(chan os.Signal)
	signal.Notify(SigQuit, syscall.SIGINT, syscall.SIGTERM)
	SigStat := make(chan os.Signal)
	signal.Notify(SigStat, syscall.SIGUSR1)

forever:
	for {
		select {
		case s := <-SigQuit:
			log.Info(fmt.Sprintf("Signal (%d) received, stopping", s))
			break forever
		case _ = <-SigStat:
			log.Info(fmt.Sprintf("Goroutines: %d", runtime.NumGoroutine()))
		}
	}
}
