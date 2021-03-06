package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/urfave/negroni"

	"github.com/ivan1993spb/snake-server/connections"
	"github.com/ivan1993spb/snake-server/handlers"
	"github.com/ivan1993spb/snake-server/middlewares"
)

const ServerName = "Snake-Server"

const (
	defaultAddress     = ":8080"
	defaultGroupsLimit = 100
	defaultConnsLimit  = 1000
)

var (
	Version = "dev"
	Build   = "dev"
)

var (
	address string

	flagEnableTLS bool
	certFile      string
	keyFile       string

	groupsLimit int
	connsLimit  int
	seed        int64

	flagJSONLog bool
	logLevel    string
)

func usage() {
	fmt.Fprint(os.Stderr, "Wellcome to snake-server!\n\n")
	fmt.Fprintf(os.Stderr, "Server version %s, build %s\n\n", Version, Build)
	fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
	flag.PrintDefaults()
}

func init() {
	flag.StringVar(&address, "address", defaultAddress, "address to serve")
	flag.BoolVar(&flagEnableTLS, "tls-enable", false, "enable TLS")
	flag.StringVar(&certFile, "tls-cert", "", "path to certificate file")
	flag.StringVar(&keyFile, "tls-key", "", "path to key file")
	flag.IntVar(&groupsLimit, "groups-limit", defaultGroupsLimit, "groups limit")
	flag.IntVar(&connsLimit, "conns-limit", defaultConnsLimit, "web-socket connections limit")
	flag.Int64Var(&seed, "seed", time.Now().UnixNano(), "random seed")
	flag.BoolVar(&flagJSONLog, "log-json", false, "use json format for logger")
	flag.StringVar(&logLevel, "log-level", "info", "set log level: panic, fatal, error, warning (warn), info or debug")
	flag.Usage = usage
	flag.Parse()
}

func logger() *logrus.Logger {
	logger := logrus.New()
	if flagJSONLog {
		logger.Formatter = &logrus.JSONFormatter{}
	} else {
		// TODO: Set up formatter with colors for windows ?
		// https://github.com/sirupsen/logrus/issues/172
	}
	if level, err := logrus.ParseLevel(logLevel); err != nil {
		logger.SetLevel(logrus.InfoLevel)
	} else {
		logger.SetLevel(level)
	}
	return logger
}

func serve(h http.Handler) error {
	if flagEnableTLS {
		return http.ListenAndServeTLS(address, certFile, keyFile, h)
	}
	return http.ListenAndServe(address, h)
}

func main() {
	logger := logger()

	logger.WithFields(logrus.Fields{
		"version": Version,
		"build":   Build,
	}).Info("wellcome to snake server!")

	logger.WithFields(logrus.Fields{
		"conns_limit":  connsLimit,
		"groups_limit": groupsLimit,
		"seed":         seed,
		"log_level":    logLevel,
	}).Info("preparing to start server")

	rand.Seed(seed)

	groupManager, err := connections.NewConnectionGroupManager(logger, groupsLimit, connsLimit)
	if err != nil {
		logger.Fatalln("cannot create connections group manager:", err)
	}

	rootRouter := mux.NewRouter()

	// Web-Socket route
	rootRouter.Path(handlers.URLRouteGameWebSocketByID).Methods(handlers.MethodGame).Handler(handlers.NewGameWebSocketHandler(logger, groupManager))

	// API routes
	apiRouter := mux.NewRouter().StrictSlash(true)
	apiRouter.Path(handlers.URLRouteGetInfo).Methods(handlers.MethodGetInfo).Handler(handlers.NewGetInfoHandler(logger, Version, Build))
	apiRouter.Path(handlers.URLRouteGetCapacity).Methods(handlers.MethodGetCapacity).Handler(handlers.NewGetCapacityHandler(logger, groupManager))
	apiRouter.Path(handlers.URLRouteCreateGame).Methods(handlers.MethodCreateGame).Handler(handlers.NewCreateGameHandler(logger, groupManager))
	apiRouter.Path(handlers.URLRouteGetGameByID).Methods(handlers.MethodGetGame).Handler(handlers.NewGetGameHandler(logger, groupManager))
	apiRouter.Path(handlers.URLRouteDeleteGameByID).Methods(handlers.MethodDeleteGame).Handler(handlers.NewDeleteGameHandler(logger, groupManager))
	apiRouter.Path(handlers.URLRouteGetGames).Methods(handlers.MethodGetGames).Handler(handlers.NewGetGamesHandler(logger, groupManager))
	// Use middlewares for API routes
	rootRouter.NewRoute().Handler(negroni.New(
		middlewares.NewRecovery(logger),
		middlewares.NewLogger(logger, "api"),
		middlewares.NewCORS(),
		negroni.Wrap(apiRouter),
	))

	n := negroni.New()
	n.Use(middlewares.NewServerInfo(ServerName, Version, Build))
	n.UseHandler(rootRouter)

	logger.WithFields(logrus.Fields{
		"address": address,
		"tls":     flagEnableTLS,
	}).Info("starting server")

	if err := serve(n); err != nil {
		logger.Fatalf("server error: %s", err)
	}
}
