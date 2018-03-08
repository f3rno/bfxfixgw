// Binary bfxfixgw is a gateway between bitfinex' websocket API and clients that
// speak the FIX protocol.
package main

import (
	"flag"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path"

	"github.com/bitfinexcom/bitfinex-api-go/v2/rest"
	"github.com/bitfinexcom/bitfinex-api-go/v2/websocket"

	"github.com/bitfinexcom/bfxfixgw/log"
	"github.com/bitfinexcom/bfxfixgw/service"
	"github.com/bitfinexcom/bfxfixgw/service/fix"
	"github.com/bitfinexcom/bfxfixgw/service/peer"

	"github.com/quickfixgo/quickfix"
	"go.uber.org/zap"
)

var (
	mdcfg  = flag.String("mdcfg", "demo_fix_marketdata.cfg", "Market data FIX configuration file name")
	ordcfg = flag.String("ordcfg", "demo_fix_orders.cfg", "Order flow FIX configuration file name")
	ws     = flag.String("ws", "wss://api.bitfinex.com/ws/2", "v2 Websocket API URL")
	rst    = flag.String("rest", "https://api.bitfinex.com/v2/", "v2 REST API URL")
	//flag.StringVar(&logfile, "logfile", "logs/debug.log", "path to the log file")
	//flag.StringVar(&configfile, "configfile", "config/server.cfg", "path to the config file")
)

var (
	FIXConfigDirectory = configDirectory()
)

func configDirectory() string {
	d := os.Getenv("FIX_SETTINGS_DIRECTORY")
	if d == "" {
		return "./config"
	}
	return d
}

// Gateway is a tunnel that enables a FIX client to talk to the bitfinex websocket API
// and vice versa.
type Gateway struct {
	logger *zap.Logger

	MarketData   *service.Service
	OrderRouting *service.Service

	factory peer.ClientFactory
}

func (g *Gateway) Start() error {
	err := g.MarketData.Start()
	if err != nil {
		return err
	}
	return g.OrderRouting.Start()
}

func (g *Gateway) Stop() {
	g.OrderRouting.Stop()
	g.MarketData.Stop()
}

func New(mdSettings, orderSettings *quickfix.Settings, factory peer.ClientFactory) (*Gateway, error) {
	g := &Gateway{
		logger:  log.Logger,
		factory: factory,
	}
	var err error
	g.MarketData, err = service.New(factory, mdSettings, fix.MarketDataService)
	if err != nil {
		log.Logger.Fatal("create market data FIX", zap.Error(err))
		return nil, err
	}
	g.OrderRouting, err = service.New(factory, orderSettings, fix.OrderRoutingService)
	if err != nil {
		log.Logger.Fatal("create order routing FIX", zap.Error(err))
		return nil, err
	}
	return g, nil
}

type NonceFactory interface {
	Create()
}

type defaultClientFactory struct {
	*websocket.Parameters
	RestURL string
	NonceFactory
}

func (d *defaultClientFactory) NewWs() *websocket.Client {
	if d.Parameters == nil {
		d.Parameters = websocket.NewDefaultParameters()
	}
	return websocket.NewWithParams(d.Parameters)
}

func (d *defaultClientFactory) NewRest() *rest.Client {
	if d.RestURL == "" {
		return rest.NewClient()
	}
	return rest.NewClientWithURL(d.RestURL)
}

func main() {
	flag.Parse()

	mdf, err := os.Open(path.Join(FIXConfigDirectory, *mdcfg))
	if err != nil {
		log.Logger.Fatal("FIX market data config", zap.Error(err))
	}
	mds, err := quickfix.ParseSettings(mdf)
	if err != nil {
		log.Logger.Fatal("parse FIX market data settings", zap.Error(err))
	}
	ordf, err := os.Open(path.Join(FIXConfigDirectory, *ordcfg))
	if err != nil {
		log.Logger.Fatal("FIX order flow config", zap.Error(err))
	}
	ords, err := quickfix.ParseSettings(ordf)
	if err != nil {
		log.Logger.Fatal("parse FIX order flow settings", zap.Error(err))
	}
	params := websocket.NewDefaultParameters()
	params.URL = *ws
	factory := &defaultClientFactory{
		Parameters: params,
		RestURL:    *rst,
	}
	g, err := New(mds, ords, factory)
	if err != nil {
		log.Logger.Fatal("could not create gateway", zap.Error(err))
	}
	err = g.Start()
	if err != nil {
		log.Logger.Fatal("start FIX", zap.Error(err))
	}

	g.logger.Info("starting stat server")

	// TODO remove profiling below for deployments
	g.logger.Error("stat server", zap.Error(http.ListenAndServe(":8080", nil)))
}
