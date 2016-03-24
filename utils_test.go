package mdns_test

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"testing"
	"time"

	"github.com/rackerlabs/mdns"
)

func TestInitConfig(t *testing.T) {
	mdns.InitConfig()

	assert(t, mdns.Conf.Version == false, "Version isn't false")
	assert(t, mdns.Conf.Debug == false, "Debug isn't false")
	assert(t, mdns.Conf.BindAddress == "127.0.0.1", "BindAddress isn't 127.0.0.1")
	assert(t, mdns.Conf.BindPort == "5354", "BindPort isn't 5354")
	assert(t, mdns.Conf.DbType == "mysql", "DbType isn't mysql")
	assert(t, mdns.Conf.DbConn == "root:password@tcp(127.0.0.1:3306)/designate", "DbConn is wrong")
}

func TestSetTestConfig(t *testing.T) {
	SetTestConfig()

	assert(t, mdns.Conf.Version == false, "Version isn't false")
	assert(t, mdns.Conf.Debug == true, "Debug isn't true")
	assert(t, mdns.Conf.BindAddress == "127.0.0.1", "BindAddress isn't 127.0.0.1")
	assert(t, mdns.Conf.BindPort == "5354", "BindPort isn't 5354")
	assert(t, mdns.Conf.DbType == "mysql", "DbType isn't mysql")
	assert(t, mdns.Conf.DbConn == "root:password@tcp(127.0.0.1:3306)/designate", "DbConn is wrong")
}

func TestLogging(t *testing.T) {
	SetUp()

	assert(t, log.GetLevel() == log.DebugLevel,
		fmt.Sprintf("Log level isn't debug it's: %s", log.GetLevel().String()))

	mdns.Conf.Debug = false
	mdns.InitLogging()
	assert(t, log.GetLevel() == log.InfoLevel,
		fmt.Sprintf("Log level isn't info it's: %s", log.GetLevel().String()))

	SetTestConfig()
}

func TestServe(t *testing.T) {
	SetUp()

	mysql := &mdns.MySQLDriver{}
	storage := mdns.Storage{Driver: mysql}
	handler := mdns.NewDefaultMdnsHandler(storage)

	go mdns.Serve("tcp", "127.0.0.1", "55555", handler)
	// Wait for a panic
	time.Sleep(3)
}

func TestListen(t *testing.T) {
	SetUp()

	go mdns.Listen()
	// Wait for a panic
	time.Sleep(3)
}
