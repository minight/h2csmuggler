package parallel

import (
	"bytes"
	"net/http"

	"github.com/minight/h2csmuggler/http2"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type res struct {
	target string
	res    *http.Response // response.Body is already read and closed and stored on body
	body   []byte
	err    error
}

func (r *res) IsNil() bool {
	return r.err == nil && r.res == nil
}

func (r *res) Log(source string) {
	log.WithFields(log.Fields{
		"body":   string(r.body),
		"target": r.target,
		"res":    r.res,
		"err":    r.err,
	}).Debugf("recieved")
	if r.err != nil {
		var uscErr http2.UnexpectedStatusCodeError
		if errors.As(r.err, &uscErr) {
			log.WithFields(log.Fields{
				"status": uscErr.Code,
				"target": r.target,
				"source": source,
			}).Errorf("unexpected status code")
		} else {
			log.WithField("target", r.target).WithError(r.err).Errorf("failed")
		}
	} else {
		log.WithFields(log.Fields{
			"status":  r.res.StatusCode,
			"headers": r.res.Header,
			"body":    len(r.body),
			"target":  r.target,
			"source":  source,
		}).Infof("success")
	}
}

type Diff struct {
	HTTP2 *res
	H2C   *res
}

type ResponseDiff struct {
	cache        map[string]*Diff
	DeleteOnShow bool // if enabled, results will be cleared from the cache once shown
}

func NewDiffer(DeleteOnShow bool) *ResponseDiff {
	return &ResponseDiff{
		cache:        make(map[string]*Diff),
		DeleteOnShow: DeleteOnShow,
	}
}

// ShowDiffH2C will show if there's a diff between the http2 and h2c responses.
// if the corresponding response is not cached, this does nothing
func (r *ResponseDiff) ShowDiffH2C(http2res *res) {
	d := r.diffH2C(http2res)
	if d.H2C == nil || d.HTTP2 == nil {
		return
	}
	r.diffHosts(d)
}

// ShowDiffHTTP2 will show if there's a diff between the http2 and h2c responses.
// if the corresponding response is not cached, this does nothing
func (r *ResponseDiff) ShowDiffHTTP2(http2res *res) {
	d := r.diffHTTP2(http2res)
	if d.H2C == nil || d.HTTP2 == nil {
		return
	}
	r.diffHosts(d)
}

func (r *ResponseDiff) diffHosts(d *Diff) {
	log.Tracef("got d: %+v", d)
	log.Tracef("r is :%+v", r)
	diff := false
	fields := log.Fields{}
	debugFields := log.Fields{}


	if d.HTTP2.err != d.H2C.err {
		diff = true
		if d.H2C.err != nil {
			fields["normal-status-code"] = d.HTTP2.res.StatusCode
			fields["normal-response-body-len"] = len(d.HTTP2.body)
			fields["host"] = d.HTTP2.res.Request.Host
			fields["h2c-error"] = d.H2C.err
		}
		if d.HTTP2.err != nil {
			fields["h2c-status-code"] = d.H2C.res.StatusCode
			fields["h2c-response-body-len"] = len(d.H2C.body)
			fields["host"] = d.H2C.res.Request.Host
			fields["normal-error"] = d.HTTP2.err
		}
	}
	if d.HTTP2.res != nil && d.H2C.res != nil {
		fields["host"] = d.H2C.res.Request.Host
		if d.HTTP2.res.StatusCode != d.H2C.res.StatusCode {
			diff = true
			fields["normal-status-code"] = d.HTTP2.res.StatusCode
			fields["h2c-status-code"] = d.H2C.res.StatusCode
		}

		if len(d.HTTP2.res.Header) != len(d.H2C.res.Header) {
			diff = true
			sharedHeaders := http.Header{}
			http2Headers := http.Header{}
			h2cHeaders := http.Header{}
			seen := map[string]struct{}{}
			for k, v := range d.HTTP2.res.Header {
				h2cv := d.H2C.res.Header.Values(k)
				if len(v) != len(h2cv) {
					for _, vv := range v {
						http2Headers.Add(k, vv)
					}
					for _, vv := range h2cv {
						h2cHeaders.Add(k, vv)
					}
				} else {
					for _, vv := range v {
						sharedHeaders.Add(k, vv)
					}
				}
				seen[k] = struct{}{}
			}

			for k, v := range d.H2C.res.Header {
				_, ok := seen[k]
				if ok {
					continue
				}

				for _, vv := range v {
					h2cHeaders.Add(k, vv)
				}
			}
			fields["normal-headers"] = http2Headers
			fields["same-headers"] = sharedHeaders
			fields["h2c-headers"] = h2cHeaders
		}

		if len(d.HTTP2.body) != len(d.H2C.body) {
			diff = true
			fields["normal-response-body-len"] = len(d.HTTP2.body)
			fields["h2c-response-body-len"] = len(d.H2C.body)
		}

		if bytes.Compare(d.HTTP2.body, d.H2C.body) != 0 {
			debugFields["normal-body"] = string(d.HTTP2.body)
			debugFields["h2c-body"] = string(d.H2C.body)
		}
	}
	if diff {
		switch log.GetLevel() {
		case log.InfoLevel:
			log.WithFields(fields).Infof("results differ")
		default:
			log.WithFields(fields).WithFields(debugFields).Debugf("results differ")
		}
	}

	if r.DeleteOnShow {
		delete(r.cache, d.HTTP2.target)
	}
}

// DiffHTTP2 will return the diff, with the provided argument as the http2 result
func (r *ResponseDiff) diffHTTP2(http2res *res) (d *Diff) {
	diff, ok := r.cache[http2res.target]
	if !ok {
		r.cache[http2res.target] = &Diff{}
		diff = r.cache[http2res.target]
	}
	diff.HTTP2 = http2res
	return diff
}

// DiffH2C will return the diff, with the provided argument as the http2 result
func (r *ResponseDiff) diffH2C(h2cres *res) (d *Diff) {
	diff, ok := r.cache[h2cres.target]
	if !ok {
		r.cache[h2cres.target] = &Diff{}
		diff = r.cache[h2cres.target]
	}
	diff.H2C = h2cres
	return diff
}
