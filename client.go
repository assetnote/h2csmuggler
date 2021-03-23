package h2csmuggler

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/assetnote/h2csmuggler/http2"
	log "github.com/sirupsen/logrus"

	"github.com/pkg/errors"
)

var (
	DefaultDialer = &net.Dialer{
		Timeout: time.Millisecond * time.Duration(5000),
		Resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{
					Timeout: time.Millisecond * time.Duration(5000),
				}
				return d.DialContext(ctx, "udp", "1.1.1.1:53")
			},
		},
	}
	DefaultTransport = &http2.Transport{
		AllowHTTP: true,
	}
)

type ConnectionOption func(c *Conn)

func ConnectionTransport(t *http2.Transport) ConnectionOption {
	return func(c *Conn) {
		c.transport = t
	}
}

func ConnectionDialer(t *net.Dialer) ConnectionOption {
	return func(c *Conn) {
		c.dialer = t
	}
}

func ConnectionMaxRetries(v int) ConnectionOption {
	return func(c *Conn) {
		c.maxRetries = v
	}
}

// NewConn will return an unitialized h2csmuggler connection.
// The first will Do will initialize the connection and perform the upgrade.
// Target must be a parsable url including protocol e.g. https://google.com
// path and port will be inferred if not provided (443:https and 80:http)
// Initialization of the connection is lazily performed to allow for the caller to customise
// the request used to upgrade the connection
func NewConn(target string, opts ...ConnectionOption) (*Conn, error) {
	var err error
	var c Conn = Conn{
		dialer:    DefaultDialer,
		transport: DefaultTransport,
	}

	c.url, err = url.Parse(target)
	if err != nil {
		return nil, err
	}

	for _, o := range opts {
		o(&c)
	}
	return &c, nil
}

// Conn encapsulates all the state needed to perform a request over h2c. Internally, this is
// a singlethreaded connection. Functions can be called concurrently, however they will block
// on the same thread. For concurrent connections, multiple Conns should be instantiated
// Initialization of the connection is lazily performed to allow for the caller to customise
// the request used to upgrade the connection
// Instantiating a Conn should be done via Client
type Conn struct {
	url        *url.URL
	dialer     *net.Dialer
	transport  *http2.Transport
	maxRetries int

	conn net.Conn
	h2c  *http2.ClientConn

	init   bool
	initmu sync.RWMutex
}

// Initialized will return whether this connection has been initialized already
func (c *Conn) Initialized() bool {
	c.initmu.RLock()
	defer c.initmu.RUnlock()
	return c.init
}

func (c *Conn) setInitialized() {
	log.WithField("url", c.url).Tracef("setting initialized for conn")
	c.initmu.Lock()
	defer c.initmu.Unlock()
	c.init = true
	log.WithField("url", c.url).Tracef("initilzied set")

}

// Close will close the underlying connections. After this is called, the struct is no
// longer safe to use
func (c *Conn) Close() {
	if c.h2c != nil {
		c.h2c.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}
}

// UpgradeOption provides manipulation of the initial upgrade request
type UpgradeOption func(o *UpgradeOptions)

var (
	DefaultConnectionHeader    = "Upgrade, HTTP2-Settings"
	DefaultUpgradeHeader       = "h2c"
	DefaultHTTP2SettingsHeader = "AAMAAABkAARAAAAAAAIAAAAA"
)

// UpgradeOptions provide manual overrides for the specific headers needed to upgrade
// the connection to h2c. Fiddling with these may result in an unsuccessful connection
type UpgradeOptions struct {
	ConnectionHeader    string
	HTTP2SettingsHeader string
	UpgradeHeader       string

	ConnectionHeaderDisabled    bool
	HTTP2SettingsHeaderDisabled bool
	UpgradeHeaderDisabled       bool
}

func DisableHTTP2SettingsHeader(val bool) UpgradeOption {
	return func(o *UpgradeOptions) {
		o.HTTP2SettingsHeaderDisabled = val
	}
}

func DisableUpgradeHeader(val bool) UpgradeOption {
	return func(o *UpgradeOptions) {
		o.UpgradeHeaderDisabled = val
	}
}

func DisableConnectionHeader(val bool) UpgradeOption {
	return func(o *UpgradeOptions) {
		o.ConnectionHeaderDisabled = val
	}
}

func SetHTTP2SettingsHeader(val string) UpgradeOption {
	return func(o *UpgradeOptions) {
		o.HTTP2SettingsHeader = val
	}
}

func SetUpgradeHeader(val string) UpgradeOption {
	return func(o *UpgradeOptions) {
		o.UpgradeHeader = val
	}
}

func SetConnectionHeader(val string) UpgradeOption {
	return func(o *UpgradeOptions) {
		o.ConnectionHeader = val
	}
}

var (
	ErrUnexpectedScheme = errors.New("Unexpected scheme for connection")
)

// CreateConn will create a net.Conn from the URL. This will choose between a tls
// and a normal tcp connection based on the url scheme
func CreateConn(t *url.URL, dialer *net.Dialer) (ret net.Conn, err error) {
	switch t.Scheme {
	case "https":
		hostport := t.Host
		if t.Port() == "" {
			hostport = fmt.Sprintf("%s:%d", t.Host, 443)
		}

		log.Tracef("establishing tls conn on: %v", hostport)
		tlsconn, err := tls.DialWithDialer(dialer, "tcp", hostport, &tls.Config{
			InsecureSkipVerify: true,
		})
		if err != nil {
			return nil, errors.Wrap(err, "Failed to dial tls")
		}
		ret = tlsconn
	case "http":
		hostport := t.Host
		if t.Port() == "" {
			hostport = fmt.Sprintf("%s:%d", t.Host, 80)
		}
		log.Tracef("establishing tcp conn on: %v", hostport)
		ret, err = dialer.Dial("tcp", hostport)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to dial tcp")
		}
	default:
		return nil, ErrUnexpectedScheme
	}
	return
}

// doUpgrade will attempt to establish a TCP connection and perform the Upgrade Request
// This will then recieve the response from the upgraded request and return it to the caller
// This may fail due to unexpected EOF, hence retries are handled at DoUpgrade
func (c *Conn) doUpgrade(req *http.Request) (*http.Response, error) {
	log.Tracef("starting doUpgrade internal")
	var err error
	log.Tracef("establishing tcp conn")
	log.WithFields(log.Fields{
		"headers": req.Header,
	}).Tracef("performing upgrade request")

	c.conn, err = CreateConn(c.url, c.dialer)
	if err != nil {
		return nil, errors.Wrap(err, "h2csmuggler: connection failed")
	}

	cc, res, err := c.transport.H2CUpgradeRequest(req, c.conn)
	if err != nil {
		return nil, errors.Wrap(err, "h2csmuggler: upgrade failed")
	}
	c.h2c = cc
	c.setInitialized()
	return res, nil
}

// DoUpgrade will perform the request and upgrade the connection to http2 h2c.
// DoUpgrade can only be successfully called once. If called a second time, this will raise an error
// If unsuccessfully called, it can be called again, however its likely the same connection error
// will be returned (e.g. timeout, HTTP2 not supported etc...)
// The provided request will have the following headers added to ensure the upgrade occurs.
// Upgrade: h2c
// HTTP2-Settings: AAMAAABkAARAAAAAAAIAAAAA
// Connection: Upgrade
// These can be modified with the upgrade options however this may result in an unsuccessful connection
// TODO: make this threadsafe.
func (c *Conn) DoUpgrade(req *http.Request, opts ...UpgradeOption) (*http.Response, error) {
	log.Tracef("starting upgrade")
	if c.Initialized() {
		return nil, errors.New("h2csmuggler: already initialized")
	}

	o := &UpgradeOptions{
		HTTP2SettingsHeader: DefaultHTTP2SettingsHeader,
		ConnectionHeader:    DefaultConnectionHeader,
		UpgradeHeader:       DefaultUpgradeHeader,
	}
	for _, opt := range opts {
		opt(o)
	}

	// Clone to avoid corrupting the request after we add our headers
	req = req.Clone(req.Context())
	if o.UpgradeHeaderDisabled {
		req.Header.Del("Upgrade")
	} else {
		req.Header.Add("Upgrade", o.UpgradeHeader)
	}

	if o.ConnectionHeaderDisabled {
		req.Header.Del("Connnection")
	} else {
		req.Header.Add("Connection", o.ConnectionHeader)
	}

	if o.HTTP2SettingsHeaderDisabled {
		req.Header.Del("HTTP2-Settings")
	} else {
		req.Header.Add("HTTP2-Settings", o.HTTP2SettingsHeader)
	}

	var (
		res *http.Response
		err error
	)

	for i := 0; i < c.maxRetries+1; i++ {
		log.Tracef("attempt: %d/%d", i, c.maxRetries+1)
		res, err = c.doUpgrade(req)
		if err == nil {
			break
		}
		if err != nil {
			if !errors.Is(err, io.ErrUnexpectedEOF) {
				log.WithError(err).Tracef("recieved error")
				return nil, err
			} else {
				log.Tracef("recieved unexpected EOF")
			}
		}
		if c.conn != nil {
			c.conn.Close()
		}
	}
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (c *Conn) Do(req *http.Request) (*http.Response, error) {
	if !c.Initialized() {
		return c.DoUpgrade(req)
	}

	return c.h2c.RoundTrip(req)
}
