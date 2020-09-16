package parallel

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/minight/h2csmuggler"
	"github.com/minight/h2csmuggler/http2"
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

type res struct {
	target string
	res    *http.Response // response.Body is already read and closed and stored on body
	body   []byte
	err    error
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

func doConn(conn *h2csmuggler.Conn, target string, muts ...RequestMutation) (r res, err error) {
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
}

func RequestHeader(key string, value string) ParallelOption {
	return func(o *ParallelOptions) {
		mut := func(r *http.Request) {
			r.Header.Add(key, value)
		}
		o.RequestMutations = append(o.RequestMutations, mut)
	}
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
		log.WithField("res", r).Tracef("recieved")
		if r.err != nil {
			var uscErr http2.UnexpectedStatusCodeError
			if errors.As(r.err, &uscErr) {
				log.WithFields(log.Fields{
					"status": uscErr.Code,
					"target": r.target,
				}).Errorf("unexpected status code")
			} else {
				log.WithField("target", r.target).WithError(r.err).Errorf("failed")
			}
		} else {
			log.WithFields(log.Fields{
				"status": r.res.StatusCode,
				"body":   len(r.body),
				"target": r.target,
			}).Infof("success")
		}
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
				log.WithField("target", r.target).WithError(r.err).Errorf("failed")
			}
		} else {
			log.WithFields(log.Fields{
				"status": r.res.StatusCode,
				"body":   len(r.body),
				"target": r.target,
			}).Infof("success")
		}
	}

	// Wait for workers to cleanup
	wg.Wait()
	swg.Wait()
	return nil
}
