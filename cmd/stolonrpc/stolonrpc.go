package main

import (
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/gravitational/stolon/pkg/postgresql"
	"github.com/gravitational/trace"
	"github.com/kelseyhightower/envconfig"

	"github.com/gorilla/mux"
	"github.com/gorilla/rpc"
	"github.com/gorilla/rpc/json"
)

var (
	ErrCantParseConfig = errors.New("Can't parse config")
)

type Config struct {
	LogLevel         string `envconfig:"STOLONRPC_LOG_LEVEL"`
	Port             string `envconfig:"STOLONRPC_PORT"`
	DatabaseHost     string `envconfig:"STOLONRPC_DB_HOST"`
	DatabasePort     string `envconfig:"STOLONRPC_DB_PORT"`
	DatabaseUsername string `envconfig:"STOLONRPC_DB_USERNAME"`
}

func GetConfig() (*Config, error) {
	var config Config
	if err := envconfig.Process("", &config); err != nil {
		return nil, trace.Wrap(ErrCantParseConfig)
	}
	return &config, nil
}

func setupLogging(level string) error {
	lvl := strings.ToLower(level)

	if lvl == "debug" {
		trace.SetDebug(true)
	}

	sev, err := log.ParseLevel(lvl)
	if err != nil {
		return err
	}
	log.SetLevel(sev)
	log.SetFormatter(&log.TextFormatter{})
	log.SetOutput(os.Stdout)
	return nil
}

func main() {
	c, err := GetConfig()
	if err != nil {
		trace.Wrap(err)
	}

	if err = setupLogging(c.LogLevel); err != nil {
		trace.Wrap(err)
	}
	log.Infof("Start with config: %+v", c)

	s := rpc.NewServer()
	s.RegisterCodec(json.NewCodec(), "application/json")
	s.RegisterCodec(json.NewCodec(), "application/json;charset=UTF-8")

	dbConn := postgresql.ConnSettings{
		Host:     c.DatabaseHost,
		Port:     c.DatabasePort,
		Username: c.DatabaseUsername,
	}
	op := new(DatabaseOperation)
	op.dbConn = dbConn

	s.RegisterService(op, "DatabaseOperation")

	r := mux.NewRouter()
	r.Handle("/rpc", s)

	errChan := make(chan error, 1)
	go func() {
		errChan <- http.ListenAndServe(net.JoinHostPort("", c.Port), r)
	}()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	for {
		select {
		case err := <-errChan:
			if err != nil {
				log.Fatal(err)
			}
		case s := <-signalChan:
			log.Infof("Captured %s. Exiting...", s)
			os.Exit(0)
		}
	}
}
