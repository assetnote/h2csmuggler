package parallel

import (
	"crypto/tls"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/assetnote/h2csmuggler"
	"github.com/assetnote/h2csmuggler/http2"
	"github.com/pkg/errors"
)

const (
	DefaultConnPerHost   = 5
	DefaultParallelHosts = 10
)

type Client struct {
	MaxConnPerHost   int
	MaxParallelHosts int
}

func New() *Client {
	return &Client{}
}

// do will create a connection and perform the request. this is a convenience function
// to let us defer closing the connection and body without leaking it until the worker loop
// ends
func do(target string) (r res, err error) {
	r.target = target
	conn, err := h2csmuggler.NewConn(target, h2csmuggler.ConnectionMaxRetries(3))
	if err != nil {
		return r, errors.Wrap(err, "connect")
	}
	defer conn.Close()

	return doConn(conn, target)
}

type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

func doConn(conn Doer, target string, muts ...RequestMutation) (r res, err error) {
	r.target = target
	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		return r, errors.Wrap(err, "request creation")
	}
	for _, mut := range muts {
		mut(req)
	}

	res, err := conn.Do(req)
	if err != nil {
		return r, errors.Wrap(err, "connection do")
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return r, errors.Wrap(err, "body read")
	}

	r.body = body
	r.res = res
	return r, nil
}

type ParallelOption func(o *ParallelOptions)
type RequestMutation func(req *http.Request)
type ParallelOptions struct {
	RequestMutations []RequestMutation
	PrettyPrint      bool
}

func PrettyPrint(v bool) ParallelOption {
	return func(o *ParallelOptions) {
		o.PrettyPrint = v
	}
}

func RequestHeader(key string, value string) ParallelOption {
	return func(o *ParallelOptions) {
		mut := func(r *http.Request) {
			if key == "Host" {
				r.Host = value
			} else {
				r.Header.Add(key, value)
			}
		}
		o.RequestMutations = append(o.RequestMutations, mut)
	}
}

func RequestMethod(method string) ParallelOption {
	return func(o *ParallelOptions) {
		mut := func(r *http.Request) {
			r.Method = method
		}
		o.RequestMutations = append(o.RequestMutations, mut)
	}
}

// GetPathDiffOnHost will send the targets to the base host on both a http2 and a h2c connection
// the results will be diffed
// this will use c.MaxConnPerHost to parallelize the paths
// This assumes that the host can be connected to over h2c. This will fail if attempted
// with a host that cannot be h2c smuggled
// TODO: minimize allocations here, since we explode out a lot
func (c *Client) GetPathDiffOnHost(base string, targets []string, opts ...ParallelOption) error {
	maxConns := c.MaxConnPerHost
	if maxConns == 0 {
		maxConns = DefaultConnPerHost
	}

	// don't need to spin up 10 threads for just 2 targets
	if len(targets) < maxConns {
		maxConns = len(targets)
	}

	o := &ParallelOptions{}
	for _, opt := range opts {
		opt(o)
	}

	// validate our input
	baseurl, err := url.Parse(base)
	if err != nil {
		return errors.Wrap(err, "failed to parse base")
	}

	http2Client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}}

	// create a mutation for our HTTP2 client so it connects on the right
	// connection. We only change the URL, since thats used to dial the conn
	mutateBaseURL := func(r *http.Request) {
		r.URL.Host = baseurl.Host
	}
	http2ClientMutations := append(o.RequestMutations, mutateBaseURL)

	var wg sync.WaitGroup
	inh2c := make(chan string, maxConns)
	inhttp2 := make(chan string, maxConns)
	outh2c := make(chan res, maxConns)
	outhttp2 := make(chan res, maxConns)

	// Create our http2 worker threads
	for i := 0; i < maxConns; i++ {
		wg.Add(1)
		go func() {
			for t := range inhttp2 {
				log.WithField("target", t).Tracef("requesting")
				r, err := doConn(http2Client, t, http2ClientMutations...)
				if err != nil {
					log.WithField("target", t).WithError(err).Tracef("failed to request")
					r.err = err
				}
				outhttp2 <- r
			}

			wg.Done()
		}()
	}

	// Create our h2c worker threads
	for i := 0; i < maxConns; i++ {
		wg.Add(1)
		go func() {
			conn, connErr := h2csmuggler.NewConn(base, h2csmuggler.ConnectionMaxRetries(3))
			if connErr == nil {
				defer conn.Close()
			}

			// initialize the connection with our first base request
			r, err := doConn(conn, base, o.RequestMutations...)
			if err != nil {
				log.WithField("target", base).WithError(err).Tracef("failed to request")
				r.err = err
			}
			// don't return the result because its expected for this to work

			for t := range inh2c {
				// just discard all results if we can't connect.
				if connErr != nil {
					outh2c <- res{
						target: t,
						err:    connErr,
					}
					continue
				}

				log.WithField("target", t).Tracef("requesting")
				r, err := doConn(conn, t, o.RequestMutations...)
				if err != nil {
					log.WithField("target", t).WithError(err).Tracef("failed to request")
					r.err = err
				}
				log.Tracef("got result: %+v", r)
				outh2c <- r
			}

			wg.Done()
		}()
	}

	var swg sync.WaitGroup
	swg.Add(1)
	// Create our dispatcher thread
	go func() {
		for _, t := range targets {
			log.WithField("target", t).Tracef("scheduling")
			inhttp2 <- t
			inh2c <- t
		}
		close(inh2c)
		close(inhttp2)

		// wait for all the workers to finish, then close our respones channel
		wg.Wait()
		close(outh2c)
		close(outhttp2)
		swg.Done()
	}()

	// Fan-in results
	results := NewDiffer(true)
	results.PrettyPrint = o.PrettyPrint
	h2cClosed := false
	http2Closed := false
	for {
		if h2cClosed && http2Closed {
			break
		}
		select {
		case r := <-outh2c:
			{
				if r.IsNil() {
					h2cClosed = true
					break
				}
				tmp := r
				// r.Log("h2c")
				results.ShowDiffH2C(&tmp)
			}
		case r := <-outhttp2:
			{
				if r.IsNil() {
					http2Closed = true
					break
				}
				tmp := r
				// r.Log("http2")
				results.ShowDiffHTTP2(&tmp)
			}
		}
	}

	// Wait for workers to cleanup
	wg.Wait()
	swg.Wait()
	return nil
}

// GetPathsOnHost will send the targets to the base host
// this will use c.MaxConnPerHost to parallelize the paths
// This assumes that the host can be connected to over h2c. This will fail if attempted
// with a host that cannot be h2c smuggled
// TODO: minimize allocations here, since we explode out a lot
func (c *Client) GetPathsOnHost(base string, targets []string, opts ...ParallelOption) error {
	maxConns := c.MaxConnPerHost
	if maxConns == 0 {
		maxConns = DefaultConnPerHost
	}

	// don't need to spin up 10 threads for just 2 targets
	if len(targets) < maxConns {
		maxConns = len(targets)
	}

	o := &ParallelOptions{}
	for _, opt := range opts {
		opt(o)
	}

	// validate our input
	_, err := url.Parse(base)
	if err != nil {
		return errors.Wrap(err, "failed to parse base")
	}

	var wg sync.WaitGroup
	in := make(chan string, maxConns)
	out := make(chan res, maxConns)

	// Create our worker threads
	for i := 0; i < maxConns; i++ {
		wg.Add(1)
		go func() {
			conn, connErr := h2csmuggler.NewConn(base, h2csmuggler.ConnectionMaxRetries(3))
			if connErr == nil {
				defer conn.Close()
			}

			// initialize the connection with our first base request
			r, err := doConn(conn, base, o.RequestMutations...)
			if err != nil {
				log.WithField("target", base).WithError(err).Tracef("failed to request")
				r.err = err
			}
			// don't return the result because its expected for this to work

			for t := range in {
				// just discard all results if we can't connect.
				if connErr != nil {
					out <- res{
						target: t,
						err:    connErr,
					}
					continue
				}

				log.WithField("target", t).Tracef("requesting")
				r, err := doConn(conn, t, o.RequestMutations...)
				if err != nil {
					log.WithField("target", t).WithError(err).Tracef("failed to request")
					r.err = err
				}
				out <- r
			}

			wg.Done()
		}()
	}

	var swg sync.WaitGroup
	swg.Add(1)
	// Create our dispatcher thread
	go func() {
		for _, t := range targets {
			log.WithField("target", t).Tracef("scheduling")
			in <- t
		}
		close(in)

		// wait for all the workers to finish, then close our respones channel
		wg.Wait()
		close(out)
		swg.Done()
	}()

	// Fan-in results
	for r := range out {
		r.Log("h2c", o.PrettyPrint)
	}

	// Wait for workers to cleanup
	wg.Wait()
	swg.Wait()
	return nil
}

// GetParallelHosts will retrieve each target on a separate connection
// This uses a simple fan-out fan-in concurrency model
func (c *Client) GetParallelHosts(targets []string) error {
	maxHosts := c.MaxParallelHosts
	if maxHosts == 0 {
		maxHosts = DefaultParallelHosts
	}

	var wg sync.WaitGroup
	in := make(chan string, maxHosts)
	out := make(chan res, maxHosts)

	// Create our worker threads
	for i := 0; i < maxHosts; i++ {
		wg.Add(1)
		go func() {
			for t := range in {
				log.WithField("target", t).Tracef("requesting")
				r, err := do(t)
				if err != nil {
					log.WithField("target", t).WithError(err).Tracef("failed to request")
					r.err = err
				}
				out <- r
			}

			wg.Done()
		}()
	}

	var swg sync.WaitGroup
	swg.Add(1)
	// Create our dispatcher thread
	go func() {
		for _, t := range targets {
			log.WithField("target", t).Tracef("scheduling")
			in <- t
		}
		close(in)

		// wait for all the workers to finish, then close our respones channel
		wg.Wait()
		close(out)
		swg.Done()
	}()

	// Fan-in results
	for r := range out {
		log.WithField("res", r).Tracef("recieved")
		if r.err != nil {
			var uscErr http2.UnexpectedStatusCodeError
			if errors.As(r.err, &uscErr) {
				log.WithFields(log.Fields{
					"status": uscErr.Code,
					"target": r.target,
				}).Errorf("unexpected status code")
			} else {
				log.WithField("target", r.target).WithError(r.err).Debugf("failed")
			}
		} else {
			switch log.GetLevel() {
			case log.DebugLevel:
				log.WithFields(log.Fields{
					"status":  r.res.StatusCode,
					"body":    r.body,
					"target":  r.target,
					"headers": r.res.Header,
				}).Infof("success")
			default:
				log.WithFields(log.Fields{
					"status":  r.res.StatusCode,
					"body":    len(r.body),
					"target":  r.target,
					"headers": r.res.Header,
				}).Infof("success")
			}
		}
	}

	// Wait for workers to cleanup
	wg.Wait()
	swg.Wait()
	return nil
}
