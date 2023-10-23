package httpppc

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/pires/go-proxyproto"
)

// New returns a new HTTP Proxy Protocol RoundTripper.
// The transport tr is the base transport to wrap and add Proxy Protocol to new connections.
// The transport tr may be nil, in which case the http.DefaultTransport is used.
func New(clientIP net.IP, clientPort int, tr *http.Transport) http.RoundTripper {
	// we can't actually use http.DefaultTransport, because it doesn't expose its defaults.
	// so we create an http.Tranpsort with the documented defaults
	if tr == nil {
		dialer := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		tr = &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}
	}

	dci := &dialContextIntercepter{
		real:       tr.DialContext,
		clientIP:   clientIP,
		clientPort: clientPort,
	}
	tr.DialContext = dci.DialContext

	return &transport{
		real: tr,
	}
}

type transport struct {
	real http.RoundTripper
}

// RoundTrip implements http.RoundTripper
func (tr *transport) RoundTrip(rq *http.Request) (*http.Response, error) {
	return tr.real.RoundTrip(rq)
}

type dialContextIntercepter struct {
	real       func(ctx context.Context, network, addr string) (net.Conn, error)
	clientIP   net.IP
	clientPort int
}

func (di *dialContextIntercepter) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := di.real(ctx, network, addr)

	if err != nil {
		return conn, err
	}

	remoteAddr := conn.RemoteAddr()
	if remoteAddr == nil {
		// I don't think this is possible
		return conn, errors.New("DialContext didn't know its remote")
	}
	host, portStr, err := net.SplitHostPort(remoteAddr.String())
	if err != nil {
		return conn, errors.New("DialContext returned malformed addr '" + remoteAddr.String() + "'")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return conn, errors.New("DialContext returned malformed addr '" + remoteAddr.String() + "' port '" + portStr + "'")
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return conn, errors.New("DialContext returned malformed addr '" + remoteAddr.String() + "' host '" + host + "'")
	}

	// convert to 4 if it was a 6-formatted 4
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}

	header := &proxyproto.Header{
		Version:           1,
		Command:           proxyproto.PROXY,
		TransportProtocol: proxyproto.TCPv4,
		SourceAddr: &net.TCPAddr{
			IP:   di.clientIP,
			Port: di.clientPort,
		},
		DestinationAddr: &net.TCPAddr{
			IP:   ip,
			Port: port,
		},
	}

	// After the connection was created write the proxy headers first
	_, err = header.WriteTo(conn)

	if err != nil {
		return conn, errors.New("writing Proxy Protocol header: " + err.Error())
	}

	return conn, nil
}
