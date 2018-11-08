// Copyright 2015 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package proxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GoogleCloudPlatform/cloudsql-proxy/logging"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/metrics"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/record"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/monitoring/tag"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/recutil"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/termcode"
	octag "go.opencensus.io/tag"
)

const (
	DefaultRefreshCfgThrottle = time.Minute
	keepAlivePeriod           = time.Minute
)

// errNotCached is returned when the instance was not found in the Client's
// cache. It is an internal detail and is not actually ever returned to the
// user.
var errNotCached = errors.New("instance was not found in cache")

// Conn represents a connection from a client to a specific instance.
type Conn struct {
	// Instance is the name of the instance.
	Instance string
	// Conn is the received connection.
	Conn net.Conn
	// Start is the time connection is initiated.
	Start time.Time
}

// CertSource is how a Client obtains various certificates required for operation.
type CertSource interface {
	// Local returns a certificate that can be used to authenticate with the
	// provided instance.
	Local(ctx context.Context, instance string) (tls.Certificate, error)
	// Remote returns the instance's CA certificate, address, IP type, and name.
	Remote(ctx context.Context, instance string) (cert *x509.Certificate, addr, ipType, name string, err error)
}

// Client is a type to handle connecting to a Server. All fields are required
// unless otherwise specified.
type Client struct {
	// Port designates which remote port should be used when connecting to
	// instances. This value is defined by the server-side code, but for now it
	// should always be 3307.
	Port int
	// Required; specifies how certificates are obtained.
	Certs CertSource
	// Optionally tracks connections through this client. If nil, connections
	// are not tracked and will not be closed before method Run exits.
	Conns *ConnSet
	// Dialer should return a new connection to the provided address. It is
	// called on each new connection to an instance. net.Dial will be used if
	// left nil.
	Dialer func(net, addr string) (net.Conn, error)

	// RefreshCfgThrottle is the amount of time to wait between configuration
	// refreshes. If not set, it defaults to 1 minute.
	//
	// This is to prevent quota exhaustion in the case of client-side
	// malfunction.
	RefreshCfgThrottle time.Duration

	// The cfgCache holds the most recent connection configuration keyed by
	// instance. Relevant functions are refreshCfg and cachedCfg. It is
	// protected by cfgL.
	cfgCache map[string]cacheEntry
	cfgL     sync.RWMutex

	// MaxConnections is the maximum number of connections to establish
	// before refusing new connections. 0 means no limit.
	MaxConnections uint64

	// ConnectionsCounter is used to enforce the optional maxConnections limit
	ConnectionsCounter uint64

	// connNums holds active connection number of instances keyed by instance names.
	connNums map[string]int64
	// connNumsMu protects access to ConnNums.
	connNumsMu sync.Mutex
}

type cacheEntry struct {
	lastRefreshed time.Time
	// If err is not nil, the addr and cfg are not valid but ipType is always valid.
	err    error
	addr   string
	ipType string
	cfg    *tls.Config
}

// Run causes the client to start waiting for new connections to connSrc and
// proxy them to the destination instance. It blocks until connSrc is closed.
func (c *Client) Run(ctx context.Context, connSrc <-chan Conn) {
	for conn := range connSrc {
		go c.handleConn(ctx, conn)
	}

	if err := c.Conns.Close(); err != nil {
		logging.Errorf("closing client had error: %v", err)
	}
}

func (c *Client) handleConn(ctx context.Context, conn Conn) {
	instance := conn.Instance
	ctx, err := recutil.AddTags(ctx, instance)
	if err != nil {
		logging.Errorf("%v", err)
	}

	termCodeRec := func(ctx context.Context, termCode, ipType string) {
		err := record.Increment(ctx, metrics.TermCodeCount,
			octag.Tag{tag.TermCodeKey, termCode},
			octag.Tag{tag.IPTypeKey, ipType},
		)
		if err != nil {
			logging.Errorf("%v", recutil.Err(instance, metrics.TermCodeCount, err))
		}
	}

	// Track connections count only if a maximum connections limit is set to avoid useless overhead
	if c.MaxConnections > 0 {
		active := atomic.AddUint64(&c.ConnectionsCounter, 1)

		// Deferred decrement of ConnectionsCounter upon connection closing
		defer atomic.AddUint64(&c.ConnectionsCounter, ^uint64(0))

		if active > c.MaxConnections {
			logging.Errorf("too many open connections (max %d)", c.MaxConnections)
			termCodeRec(ctx, termcode.TooManyClientConn, recutil.Unknown)
			conn.Conn.Close()
			return
		}
	}

	server, ipType, err := c.dialImpl(ctx, instance)
	if err != nil {
		logging.Errorf("couldn't connect to %q: %v", instance, err)
		termCodeRec(ctx, termcode.GetCode(err), ipType)
		conn.Conn.Close()
		return
	}

	if false {
		// Log the connection's traffic via the debug connection if we're in a
		// verbose mode. Note that this is the unencrypted traffic stream.
		conn.Conn = dbgConn{conn.Conn}
	}

	ipTypeTag := octag.Tag{tag.IPTypeKey, ipType}
	if err := record.SetDuration(ctx, metrics.ConnLatency, time.Now().Sub(conn.Start), ipTypeTag); err != nil {
		logging.Errorf("%v", recutil.Err(instance, metrics.ConnLatency, err))
	}
	if err := record.SetVal(ctx, metrics.ActiveConnNum, c.connNumInc(instance), ipTypeTag); err != nil {
		logging.Errorf("%v", recutil.Err(instance, metrics.ActiveConnNum, err))
	}

	c.Conns.Add(instance, conn.Conn)
	termCode := copyThenClose(ctx, ipTypeTag, server, conn.Conn, instance, "local connection on "+conn.Conn.LocalAddr().String())
	termCodeRec(ctx, termCode, ipType)

	if err := record.SetVal(ctx, metrics.ActiveConnNum, c.connNumDec(instance), ipTypeTag); err != nil {
		logging.Errorf("%v", recutil.Err(instance, metrics.ActiveConnNum, err))
	}
	if err := c.Conns.Remove(instance, conn.Conn); err != nil {
		logging.Errorf("%s", err)
	}
}

// connNumInc increments the active connection number of instance.
func (c *Client) connNumInc(instance string) int64 {
	c.connNumsMu.Lock()
	defer c.connNumsMu.Unlock()
	if c.connNums == nil {
		c.connNums = make(map[string]int64)
	}
	c.connNums[instance]++
	return c.connNums[instance]
}

// connNumDec decrements the active connection number of instance. It is expected that corresponding
// connNumInc was called before this method is called.
func (c *Client) connNumDec(instance string) int64 {
	c.connNumsMu.Lock()
	defer c.connNumsMu.Unlock()
	c.connNums[instance]--
	return c.connNums[instance]
}

// refreshCfg uses the CertSource inside the Client to find the instance's
// address as well as construct a new tls.Config to connect to the instance. It
// caches the result.
func (c *Client) refreshCfg(ctx context.Context, instance string) (addr, ipType string, cfg *tls.Config, err error) {
	c.cfgL.Lock()
	defer c.cfgL.Unlock()

	throttle := c.RefreshCfgThrottle
	if throttle == 0 {
		throttle = DefaultRefreshCfgThrottle
	}

	if old := c.cfgCache[instance]; time.Since(old.lastRefreshed) < throttle {
		logging.Errorf("Throttling refreshCfg(%s): it was only called %v ago", instance, time.Since(old.lastRefreshed))
		if err := record.Increment(ctx, metrics.TLSCfgRefreshThrottleCount); err != nil {
			logging.Errorf("%v", recutil.Err(instance, metrics.TLSCfgRefreshThrottleCount, err))
		}
		// Refresh was called too recently, just reuse the result.
		return old.addr, old.ipType, old.cfg, old.err
	}

	if c.cfgCache == nil {
		c.cfgCache = make(map[string]cacheEntry)
	}

	defer func() {
		c.cfgCache[instance] = cacheEntry{
			lastRefreshed: time.Now(),

			err:    err,
			addr:   addr,
			ipType: ipType,
			cfg:    cfg,
		}
	}()

	mycert, err := c.Certs.Local(ctx, instance)
	if err != nil {
		ipType = recutil.Unknown
		return "", ipType, nil, err
	}

	scert, addr, ipType, name, err := c.Certs.Remote(ctx, instance)
	if err != nil {
		return "", ipType, nil, err
	}
	certs := x509.NewCertPool()
	certs.AddCert(scert)

	cfg = &tls.Config{
		ServerName:   name,
		Certificates: []tls.Certificate{mycert},
		RootCAs:      certs,
		// We need to set InsecureSkipVerify to true due to
		// https://github.com/GoogleCloudPlatform/cloudsql-proxy/issues/194
		// https://tip.golang.org/doc/go1.11#crypto/x509
		//
		// Since we have a secure channel to the Cloud SQL API which we use to retrieve the
		// certificates, we instead need to implement our own VerifyPeerCertificate function
		// that will verify that the certificate is OK.
		InsecureSkipVerify:    true,
		VerifyPeerCertificate: genVerifyPeerCertificateFunc(name, certs),
	}
	return fmt.Sprintf("%s:%d", addr, c.Port), ipType, cfg, nil
}

// genVerifyPeerCertificateFunc creates a VerifyPeerCertificate func that verifies that the peer
// certificate is in the cert pool. We need to define our own because of our sketchy non-standard
// CNs.
func genVerifyPeerCertificateFunc(instanceName string, pool *x509.CertPool) func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return fmt.Errorf("no certificate to verify")
		}

		cert, err := x509.ParseCertificate(rawCerts[0])
		if err != nil {
			return fmt.Errorf("x509.ParseCertificate(rawCerts[0]) returned error: %v", err)
		}

		opts := x509.VerifyOptions{Roots: pool}
		if _, err = cert.Verify(opts); err != nil {
			return err
		}

		if cert.Subject.CommonName != instanceName {
			return fmt.Errorf("certificate had CN %q, expected %q", cert.Subject.CommonName, instanceName)
		}
		return nil
	}
}

func (c *Client) cachedCfg(instance string) (addr, ipType string, cfg *tls.Config) {
	c.cfgL.RLock()
	ret, ok := c.cfgCache[instance]
	c.cfgL.RUnlock()

	// Don't waste time returning an expired/invalid cert.
	if !ok || ret.err != nil || time.Now().After(ret.cfg.Certificates[0].Leaf.NotAfter) {
		return "", recutil.Unknown, nil
	}
	return ret.addr, ret.ipType, ret.cfg
}

// Dial uses the configuration stored in the client to connect to an instance.
// If this func returns a nil error the connection is correctly authenticated
// to connect to the instance.
func (c *Client) Dial(instance string) (net.Conn, error) {
	conn, _, err := c.dialImpl(context.Background(), instance)
	return conn, err
}

// dialImpl is the implementation of Dial that also returns IP type of the connected instance.
func (c *Client) dialImpl(ctx context.Context, instance string) (conn net.Conn, ipType string, err error) {
	if addr, ipType, cfg := c.cachedCfg(instance); cfg != nil {
		ret, err := c.tryConnect(addr, cfg)
		if err == nil {
			return ret, ipType, err
		}
	}

	addr, ipType, cfg, err := c.refreshCfg(ctx, instance)
	if err != nil {
		return nil, ipType, err
	}
	conn, err = c.tryConnect(addr, cfg)
	return conn, ipType, err
}

func (c *Client) tryConnect(addr string, cfg *tls.Config) (net.Conn, error) {
	d := c.Dialer
	if d == nil {
		d = net.Dial
	}
	conn, err := d("tcp", addr)
	if err != nil {
		return nil, termcode.Error{termcode.DialErr, err}
	}
	type setKeepAliver interface {
		SetKeepAlive(keepalive bool) error
		SetKeepAlivePeriod(d time.Duration) error
	}

	if s, ok := conn.(setKeepAliver); ok {
		if err := s.SetKeepAlive(true); err != nil {
			logging.Verbosef("Couldn't set KeepAlive to true: %v", err)
		} else if err := s.SetKeepAlivePeriod(keepAlivePeriod); err != nil {
			logging.Verbosef("Couldn't set KeepAlivePeriod to %v", keepAlivePeriod)
		}
	} else {
		logging.Verbosef("KeepAlive not supported: long-running tcp connections may be killed by the OS.")
	}

	ret := tls.Client(conn, cfg)
	if err := ret.Handshake(); err != nil {
		ret.Close()
		return nil, termcode.Error{termcode.TLSHandshakeErr, err}
	}
	return ret, nil
}

// Listen listens for connection to the instace and pass successful connections with their contexts
// for recording to handleConn. address is the address that listen listens. This function is
// blocking and returns when listener has error on accepting connection. Note that this funcion does
// not close the listener on return.
func Listen(ctx context.Context, handleConn func(context.Context, Conn), l net.Listener, instance, address string) {
	ctx, err := recutil.AddTags(ctx, instance)
	if err != nil {
		logging.Errorf("%v", err)
	}

	for {
		listenStart := time.Now()
		c, listenErr := l.Accept()
		connStart := time.Now()

		// We have a connection requst. Record it with the proxy client version
		// information to track.
		if recErr := record.SetStr(ctx, metrics.Version, recutil.Version); recErr != nil {
			logging.Errorf("%v", recutil.Err(instance, metrics.Version, recErr))
		}
		if recErr := record.Increment(ctx, metrics.ConnReqCount); recErr != nil {
			logging.Errorf("%v", recutil.Err(instance, metrics.ConnReqCount, recErr))
		}

		if listenErr != nil {
			logging.Errorf("listener error on accepting connection for instance %s on %s: %v", instance, address, listenErr)
			// Retry on temporary error.
			if nerr, ok := listenErr.(net.Error); ok && nerr.Temporary() {
				if d := 10*time.Millisecond - time.Since(listenStart); d > 0 {
					time.Sleep(d)
				}
				continue
			}

			// Failed to get the connection request. Record it and exit the function.
			recErr := record.Increment(ctx, metrics.TermCodeCount,
				octag.Tag{tag.TermCodeKey, termcode.ListenerErr},
				octag.Tag{tag.IPTypeKey, recutil.Unknown},
			)
			if recErr != nil {
				logging.Errorf("%v", recutil.Err(instance, metrics.TermCodeCount, recErr))
			}
			return
		}

		// We have a successful connection request. Pass it to the handle.
		conn := Conn{
			Instance: instance,
			Conn:     c,
			Start:    connStart,
		}
		handleConn(ctx, conn)
	}
}

// NewConnSrc returns a chan which can be used to receive connections
// on the passed Listener. All requests sent to the returned chan will have the
// instance name provided here. The chan will be closed if the Listener returns
// an error.
func NewConnSrc(instance string, l net.Listener) <-chan Conn {
	ctx := context.Background()
	ch := make(chan Conn)
	handleConn := func(_ context.Context, conn Conn) {
		ch <- conn
	}
	go func() {
		Listen(ctx, handleConn, l, instance, fmt.Sprintf("listener (%#v)", l))
		l.Close()
		close(ch)
	}()
	return ch
}
