package probes

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/mt-inside/y-u-no-internetz/pkg/permissions"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

func Ping(ctx context.Context, log logr.Logger, period time.Duration) {
	target := "1.1.1.1"

	log = log.WithName("ping").WithValues("target", target)

	c, targetAddr, err := makePingSocket(log, target)
	if err != nil {
		log.Error(err, "Coundn't make a socket for ping operations")
		return
	}
	defer c.Close()

	tick := time.NewTicker(period)
	for range tick.C {
		ping := icmp.Message{
			Type: ipv4.ICMPTypeEcho,
			Code: 0,
			Body: &icmp.Echo{
				ID:   os.Getpid() & 0xffff,
				Seq:  42,
				Data: []byte("spam"),
			},
		}
		pingb, err := ping.Marshal(nil)
		if err != nil {
			log.Error(err, "Couldn't construct icmp echo request packet")
			return
		}

		if _, err := c.WriteTo(pingb, targetAddr); err != nil {
			log.Error(err, "Couldn't send icmp echo request")
		}

		pongb := make([]byte, 1500)
		n, peer, err := c.ReadFrom(pongb)
		if err != nil {
			log.Error(err, "Counldn't receive icmp echo reply")
			return
		}
		if peerHost, _, _ := net.SplitHostPort(peer.String()); peerHost != target {
			log.Error(fmt.Errorf("Target was %s but peer reported as %s", target, peer), "Reply from unexpected place")
			return
		}
		pong, err := icmp.ParseMessage(ipv4.ICMPTypeEchoReply.Protocol(), pongb[:n])
		if err != nil {
			log.Error(err, "Couldn't parse icmp echo reply packet")
		}

		switch pong.Type {
		case ipv4.ICMPTypeEchoReply:
			log.Info("ok")
		default:
			log.Error(errors.New("Expecting icmp echo reply"), "Unknown ICMP packet type", "got", pong)
		}
	}
}

func makePingSocket(log logr.Logger, targetIP string) (*icmp.PacketConn, net.Addr, error) {
	var targetAddr net.Addr

	/* First, try a "ping socket". These are a special mode of socket that allow only limited ICMP echo request and reply transactions, but require no privleges, bar our GID being in the range in net.ipv4.ping_group_range
	 */
	c, err := icmp.ListenPacket("udp4", "0.0.0.0") // "ping-sockets" are implemented as PF_INET, SOCK_DGRAM, PROT_ICMP
	if err != nil {
		log.Info("couldn't make ping-socket (dgram) icmp socket, falling back to raw socket. Check that this process's group id is within the range in net.ipv4.ping_group_range", "primary group", os.Getgid(), "error", err)
	} else {
		targetAddr = &net.UDPAddr{IP: net.ParseIP(targetIP)}
	}

	if c == nil {
		permissions.ApplyNetRaw(log)
		defer permissions.DropNetRaw(log)

		c, err = icmp.ListenPacket("ip4:icmp", "0.0.0.0")
		if err != nil {
			return nil, nil, fmt.Errorf("couldn't make raw icmp socket: %w", err)
		} else {
			targetAddr = &net.IPAddr{IP: net.ParseIP(targetIP)}
		}
	}

	return c, targetAddr, nil
}
