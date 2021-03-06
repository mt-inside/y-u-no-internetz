package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/mattn/go-isatty"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/mt-inside/y-u-no-internetz/pkg/probes"
)

var (
	devMode bool
)

func init() {
	flag.BoolVar(&devMode, "dev", false, "(optional) Run in dev-friendly mode, eg pretty-printed logs")
}

func main() {
	flag.Parse() // TODO use mow or cobra or something

	log := getLogger()
	log.Info("Starting")

	signalCh := installSignalHandlers(log)

	printNetInfo(log)

	/* on that observable thing:
	* * make a table of what return, panic(), os.exit, etc do on background goroutines
	* * can i `foo = go lol()` or `go foo = lol()`?
	* what's the pattern for catching / restarting these things? How to impliment erlang?
	* EN what recover does, point to demo repo (push it to GH)
	* look at rxgo
	* build own observable pattern for an example
	* think about what this prog actually needs
	* - don't really want to bring all results back to main thread? each package should manage its own prom and errors (knows what's transient and what's not)
	* - don't want a startup error in one loop to quit the whole thing, just log and let that goroutine return so that one doesn't run
	* - don't want a run-time error in one loop to stop the whole thing, just try to recover
	* - so actually this doesn't want observables, but still play with the pattern in an example
	* - but there is a stream of events that each probe has to deal with
	*   - value(ok) - all good; don't log, metrics
	*   - value(down) - expected, transient; don't crash, log (in the sense of user interface), metrics
	*   - error - unexpected, permanent, eg no permissions to make socket, can't set read deadline; perform graceful shutdown (to alert a human); log at error
	*   - done - you asked me to gracefully shutdown and how I'm done - you can block waiting for this value
	*   - so yanno, thread->main needs error and done ch, like the graceful shutdown example. It probably does also wanna send values, cause it can go through an metrics middleware, which just don't pass them to main (use RxGo)
	* design the algorithm for sending on the second, matching up replies, raising an error if they don't return within n (irellevant of other replies overtaking them)
	* - think about it yourself
	*   - initial idea: send each req on a goroutine, spawn them on the second, pass them a 5s ctx and use that with an async or cancellable recv()
	* - look at go-ping/ping for inspiration
	* - this is your observable stream (which might stay local) - Result(good) if there's a reply withing 5, Result(bad) if 5s expires (this is expected and NOT and error case), Err if there's a problem sending or whatever, maybe a laptop's wifi goes down, the trick will be in separating that from internet failures, Done if the stop context is popped
	 */

	ctx, cancel := context.WithCancel(context.Background())

	go probes.Stream()

	//TODO: these things should return a done channel, which graceful shutdown can wait for (see graceful shutdown ex; they're just not gonna send errors down it)
	go probes.Tcp(ctx, log, 1*time.Second)

	go probes.Udp(ctx, log, 1*time.Second)

	go probes.Ping(ctx, log, 1*time.Second)

	// TODO renane to Dns.
	// TODO actualy make sure it *doesn't* recurse - we just wanna know if our connection to that server is up, not if it can reach other stuff
	go probes.RecursiveDns(ctx, log, 1*time.Second)

	go probes.Http(ctx, log, 1*time.Second)

	log.Info("Running")
	<-signalCh // this is the only thing that stops this programme; all the check loops swallow errors and try to recover

	log.Info("Shutting down")
	cancel()

	log.Info("Done")
}

type ZaprWriter struct{ log logr.Logger }

func (w ZaprWriter) Write(data []byte) (n int, err error) {
	w.log.Info(string(data))
	return len(data), nil
}

func getLogger() logr.Logger {
	var zapLog *zap.Logger
	var err error

	if isatty.IsTerminal(os.Stdout.Fd()) || devMode {
		c := zap.NewDevelopmentConfig()
		c.EncoderConfig.EncodeCaller = nil
		c.EncoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(t.Format("15:04:05"))
		}
		zapLog, err = c.Build()
	} else {
		zapLog, err = zap.NewProduction()
	}
	if err != nil {
		panic(err.Error())
	}

	zr := zapr.NewLogger(zapLog)

	if devMode {
		zr.Info("Logging in dev mode; remove --dev flag for structured json output")
	}

	log.SetFlags(0) // don't add date and timestamps to the message, as the zapr writer will do that
	log.SetOutput(ZaprWriter{zr.WithValues("source", "go log")})

	return zr
}

func installSignalHandlers(log logr.Logger) <-chan struct{} {
	stopCh := make(chan struct{})
	signalCh := make(chan os.Signal, 2)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-signalCh
		log.Info("Got signal", "signal", sig)
		close(stopCh)

		sig = <-signalCh
		log.Info("Got signal", "signal", sig)
		os.Exit(1) // user is insistent
	}()

	return stopCh
}

func printNetInfo(log logr.Logger) {
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Error(err, "Can't show system network interfaces")
		return
	}

	for _, iface := range ifaces {
		if ifaceExclude(iface) {
			continue
		}

		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			if addr.(*net.IPNet).IP.To4() == nil {
				continue
			}
			log.Info("Interface", "number", iface.Index, "name", iface.Name, "addr", addr.String(), "flags", iface.Flags)
		}
	}

	log.Info("Default iface", "addr", getDefaultIP())

	log.Info("External IP and netblock's AS etc", "details", "TODO")
}

func ifaceExclude(iface net.Interface) bool {
	if strings.HasPrefix(iface.Name, "veth") {
		return true
	} else if strings.HasPrefix(iface.Name, "lo") {
		return true
	}

	return false
}

func getDefaultIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:53")
	if err != nil {
		log.Println(err)
		return "<unknown>"
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP.String()
}
