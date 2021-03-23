package parallel

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httputil"

	"github.com/assetnote/h2csmuggler/http2"
	jsoniter "github.com/json-iterator/go"
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

func (r *res) Log(source string, PrettyPrint bool) {
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
		if PrettyPrint {
			fmt.Printf("[H2C Smuggling detected on %s]\n", r.target)
			if r.err == nil {
				fmt.Println("[Smuggled response]")
				httpr, err := httputil.DumpResponse(r.res, false)
				if err != nil {
					log.WithError(err).Errorf("failed to dump h2c rponse")
				}
				fmt.Printf("%s", string(httpr))
				if log.GetLevel() != log.InfoLevel {
					fmt.Println(string(r.body))
				} else {
					fmt.Printf("[Response Body: %d bytes]\n", len(r.body))
				}
			} else {
				fmt.Println("[Smuggled Error]")
				fmt.Println(r.err)
				fmt.Println()
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
}

type Diff struct {
	HTTP2 *res
	H2C   *res
}

type ResponseDiff struct {
	cache        map[string]*Diff
	DeleteOnShow bool // if enabled, results will be cleared from the cache once shown
	PrettyPrint  bool // print the diff prettily
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

type State struct {
	StatusCode         int         `json:"status_code,omitempty"`
	ResponseBodyLength int         `json:"response_body_length,omitempty"`
	Headers            http.Header `json:"headers,omitempty"`
	Body               string      `json:"body,omitempty"`
	Error              error       `json:"error,omitempty"`
}

func (s State) String() string {
	ret, err := jsoniter.ConfigFastest.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(ret)
}

type DiffState struct {
	Host        string      `json:"host,omitempty"`
	SameHeaders http.Header `json:"same_headers,omitempty"`
	H2C         State       `json:"H2C"`
	Normal      State       `json:"normal"`
}

func (d DiffState) String() string {
	ret, err := jsoniter.ConfigFastest.Marshal(d)
	if err != nil {
		panic(err)
	}
	return string(ret)
}

func (d DiffState) Map() map[string]interface{} {
	return map[string]interface{}{
		"host":                 d.Host,
		"same-headers":         d.SameHeaders,
		"h2c-status-code":      d.H2C.StatusCode,
		"h2c-resp-body-len":    d.H2C.ResponseBodyLength,
		"h2c-headers":          d.H2C.Headers,
		"h2c-error":            d.H2C.Error,
		"normal-status-code":   d.Normal.StatusCode,
		"normal-resp-body-len": d.Normal.ResponseBodyLength,
		"normal-headers":       d.Normal.Headers,
		"normal-error":         d.Normal.Error,
	}
}

func (r *ResponseDiff) diffHosts(d *Diff) {
	log.Tracef("got d: %+v", d)
	log.Tracef("r is :%+v", r)
	diff := false
	fields := log.Fields{}
	debugFields := log.Fields{}

	var res DiffState

	if d.HTTP2.err != d.H2C.err {
		diff = true
		if d.H2C.err != nil {
			res.Normal.StatusCode = d.HTTP2.res.StatusCode
			res.Normal.ResponseBodyLength = len(d.HTTP2.body)
			res.Host = d.HTTP2.res.Request.Host
			res.H2C.Error = d.H2C.err
		}
		if d.HTTP2.err != nil {
			res.H2C.StatusCode = d.H2C.res.StatusCode
			res.H2C.ResponseBodyLength = len(d.H2C.body)
			res.Host = d.H2C.res.Request.Host
			res.Normal.Error = d.HTTP2.err
		}
	}
	if d.HTTP2.res != nil && d.H2C.res != nil {
		res.Host = d.H2C.res.Request.Host
		if d.HTTP2.res.StatusCode != d.H2C.res.StatusCode {
			diff = true
			res.Normal.StatusCode = d.HTTP2.res.StatusCode
			res.H2C.StatusCode = d.H2C.res.StatusCode
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
			res.Normal.Headers = http2Headers
			res.SameHeaders = sharedHeaders
			res.H2C.Headers = h2cHeaders
		}

		if len(d.HTTP2.body) != len(d.H2C.body) {
			diff = true
			res.Normal.ResponseBodyLength = len(d.HTTP2.body)
			res.H2C.ResponseBodyLength = len(d.H2C.body)
		}

		if bytes.Compare(d.HTTP2.body, d.H2C.body) != 0 {
			res.Normal.Body = string(d.HTTP2.body)
			debugFields["normal-body"] = res.Normal.Body
			res.H2C.Body = string(d.H2C.body)
			debugFields["h2c-body"] = res.H2C.Body
		}
	}

	for k, v := range res.Map() {
		if v == nil {
			continue
		}
		if vv, ok := v.(http.Header); ok && len(vv) == 0 {
			continue
		}
		fields[k] = v
	}

	log.WithFields(fields).Tracef("Diff: %v", diff)
	if diff {
		log.Tracef("printing results: pretty(%v)", r.PrettyPrint)
		if r.PrettyPrint {
			fmt.Printf("[H2C Smuggling detected on %s]\n", res.Host)
			if d.H2C.err == nil {
				fmt.Println("[Smuggled Response]")
				r, err := httputil.DumpResponse(d.H2C.res, false)
				if err != nil {
					log.WithError(err).Errorf("failed to dump h2c response")
				}
				fmt.Printf("%s", string(r))
				if log.GetLevel() != log.InfoLevel {
					fmt.Println(string(d.H2C.body))
				} else {
					fmt.Printf("[Smuggled response body: %d bytes]\n", len(string(d.H2C.body)))
				}
				fmt.Println()
			} else {
				fmt.Println("[Smuggled Error]")
				fmt.Println(d.H2C.err)
				fmt.Println()
			}

			if d.HTTP2.err == nil {
				fmt.Println("[Normal Response]")
				r, err := httputil.DumpResponse(d.HTTP2.res, false)
				if err != nil {
					log.WithError(err).Errorf("failed to dump h2c response")
				}
				fmt.Printf("%s", string(r))
				if log.GetLevel() != log.InfoLevel {
					fmt.Println(string(d.HTTP2.body))
				} else {
					fmt.Printf("[Smuggled response body: %d bytes]\n", len(string(d.HTTP2.body)))
				}
			} else {
				fmt.Println("[Normal Error]")
				fmt.Println(d.HTTP2.err)
			}

		} else {
			switch log.GetLevel() {
			case log.InfoLevel:
				log.WithFields(fields).Infof("results differ")
			default:
				log.WithFields(fields).WithFields(debugFields).Debugf("results differ")
			}
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
