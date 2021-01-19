package probes

import (
	"context"
	"net"
	"net/http"
	"net/http/httptrace"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-ping/ping"
)

/* for the async ones, rip off the general structure of go-ping -->
* mix with observables
*
* for the sync ones - the connection may take longer than period, and there may be crazy jitter, so do async connections and deal same way as UDP etc
* make sure connections are *initiated* every second on the second
 */

/* Tests the three-way handshake */
func Tcp(ctx context.Context, log logr.Logger, period time.Duration) {
	log = log.WithName("tcp")

	tick := time.NewTicker(period)
	for range tick.C {
		conn, err := net.Dial("tcp", "1.1.1.1:80")
		if err != nil {
			log.Error(err, "down")
		} else {
			conn.Close()
			log.Info("ok")
		}
	}
}

/* FIXME: actually need to send something. Need a UDP echo service running somewhere (or just hit a DNS server and expect an error packet).
* Need to tie requests to replies, so we know when it fails. Send an int to the far end, expect it to come back within 5s? How to model that?
 */
func Udp(ctx context.Context, log logr.Logger, period time.Duration) {
	log = log.WithName("udp")

	tick := time.NewTicker(period)
	for range tick.C {
		conn, err := net.Dial("udp", "1.1.1.1:53")
		if err != nil {
			log.Error(err, "down")
		} else {
			conn.Close()
			log.Info("ok - false result!")
		}
	}
}

func Stream() {
}

/* FIXME: what to do when we don't get a reply? They have a method for that? Hook OnSend? If no, see UDP.
* Consider ripping this library off, there's no too much to it. */
func Icmp(ctx context.Context, log logr.Logger, period time.Duration) {
	log = log.WithName("ping")

	defer func() {
		if r := recover(); r != nil {
			log.Info("Recovered")
		}
	}()

	pinger, err := ping.NewPinger("1.1.1.1")
	if err != nil {
		log.Error(err, "Couldn't set up pinger")
	}
	pinger.SetNetwork("ip4")
	pinger.SetPrivileged(true) // Direct mapping true => ICMP
	pinger.Interval = period

	pinger.OnRecv = func(p *ping.Packet) {
		log.Info("ok", "latency", p.Rtt)
	}

	err = pinger.Run()
	if err != nil {
		log.Error(err, "Failure when pinging. This is assumed non-transient; no more pings will be attempted")
	}
}

func Http(ctx context.Context, log logr.Logger, period time.Duration) {
	log = log.WithName("http")

	tick := time.NewTicker(period)
	for range tick.C {
		var reqStart, connStart, connDone, wroteReq, firstByte, reqDone time.Time

		trans := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
				DualStack: false,
			}).DialContext,
			ForceAttemptHTTP2:     false,
			MaxIdleConns:          100,
			IdleConnTimeout:       10 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		trace := &httptrace.ClientTrace{
			ConnectStart:         func(network, addr string) { connStart = time.Now() },
			ConnectDone:          func(networ, addr string, err error) { connDone = time.Now() },
			WroteRequest:         func(reqInfo httptrace.WroteRequestInfo) { wroteReq = time.Now() },
			GotFirstResponseByte: func() { firstByte = time.Now() },
		}
		ctx = httptrace.WithClientTrace(ctx, trace)

		req, _ := http.NewRequestWithContext(ctx, "HEAD", "http://172.217.169.68/robots.txt", nil)

		reqStart = time.Now()
		resp, err := trans.RoundTrip(req)
		if err != nil || resp.StatusCode != 200 {
			log.Error(
				err,
				"down",
				"conn start", connStart.Sub(reqStart),
				"conn done", connDone.Sub(reqStart),
				"wrote req", wroteReq.Sub(reqStart),
				"1st byte", firstByte.Sub(reqStart),
			)
		} else {
			reqDone = time.Now()
			resp.Body.Close()
			log.Info(
				"ok",
				"conn start", connStart.Sub(reqStart),
				"conn done", connDone.Sub(reqStart),
				"wrote req", wroteReq.Sub(reqStart),
				"1st byte", firstByte.Sub(reqStart),
				"done", reqDone.Sub(reqStart),
			)
		}
		cancel() // TODO factor method
	}
}

func RecursiveDns(ctx context.Context, log logr.Logger, period time.Duration) {
	log = log.WithName("dns")

	getResolver := func(host string) *net.Resolver {
		return &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network string, addr string) (net.Conn, error) {
				d := net.Dialer{}
				return d.DialContext(ctx, "udp", host+":53")
			},
		}
	}

	/* TODO: find and print default resolver address */
	rs := map[string]*net.Resolver{
		"default": net.DefaultResolver,
		"cpe":     getResolver("192.168.69.1"),
		"ISP 1":   getResolver("188.172.144.120"),
		"ISP 2":   getResolver("141.0.144.64"),
		"CF":      getResolver("1.1.1.1"),
		"goog":    getResolver("8.8.8.8"),
	}

	// TODO seeing errros from CF and the CPE saying
	//   lookup www.google.com on [2a01:4b00:87fc:d000:7e:17ff:fe7e:5100]:53: dial udp 1.1.1.1:53: i/o timeout
	// what does this even mean? (read the go stdlib source)
	// 2a01:4b00:: - owned by hyperoptic. Are they intercepting?
	// can we avoid by
	// - tunneling dns to barnard?
	// - using v6 resolver address
	// - using TCP / encrypted etc?
	// - test-ipv6.com
	// - google guy's replies, and update him. Ask terin

	tick := time.NewTicker(period)
	for range tick.C {
		for a, r := range rs {
			log := log.WithValues("server", a)

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)

			ts := time.Now()
			_, err := r.LookupHost(ctx, "www.google.com")
			latency := time.Since(ts)

			cancel() // factor out so we can defer this
			if err != nil {
				log.Error(err, "down")
			} else {
				log.Info("ok", "latency", latency)
			}
		}
	}
}
