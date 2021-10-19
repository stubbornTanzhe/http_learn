package main_test

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

var HttpClient = &fasthttp.Client{
	MaxConnsPerHost:           8192,
	MaxIdleConnDuration:       5 * time.Second,
	ReadTimeout:               3 * time.Second,
	MaxIdemponentCallAttempts: 1,
	TLSConfig:                 &tls.Config{InsecureSkipVerify: true},
}

func setBodyBytes(req *fasthttp.Request, data []byte) {
	req.SetBody(data)
	req.Header.SetContentLength(len(data))
}

type Timeout time.Duration
type param struct {
	*fasthttp.Args
}
type Header map[string]string
type ContentType string
type QueryParam map[string]interface{}
type DisableHeaderNamesNormalizing bool

func (p *param) adds(m map[string]interface{}) {
	for k, v := range m {
		switch v := v.(type) {
		case int64, int:
			p.Add(k, fmt.Sprint(v))
		case string:
			p.Add(k, v)
		case []byte:
			p.AddBytesV(k, v)
		default:
			p.Add(k, fmt.Sprint(v))
		}
	}
}
func HttpDo(method, rawurl string, vs ...interface{}) ([]byte, error) {
	if rawurl == "" {
		err := errors.New("url not specified")
		return nil, err
	}

	var timeout Timeout

	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	req.Header.SetMethod(method)

	queryParam := param{
		Args: fasthttp.AcquireArgs(),
	}

	defer fasthttp.ReleaseArgs(queryParam.Args)
	for _, v := range vs {
		switch vv := v.(type) {
		case Timeout:
			timeout = vv
		case Header:
			for key, value := range vv {
				req.Header.Add(key, value)
			}
		case ContentType:
			req.Header.SetContentType(string(vv))
		case QueryParam:
			queryParam.adds(vv)
		case string:
			setBodyBytes(req, []byte(vv))
		case []byte:
			setBodyBytes(req, vv)
		case bytes.Buffer:
			setBodyBytes(req, vv.Bytes())
		case *fasthttp.Cookie:
			req.Header.SetCookie(string(vv.Key()), string(vv.Value()))
		case DisableHeaderNamesNormalizing:
			if vv {
				req.Header.DisableNormalizing()
			}
		case error:
			return nil, vv
		}
	}

	if queryParam.Len() != 0 {
		paramStr := queryParam.String()
		if strings.IndexByte(rawurl, '?') == -1 {
			rawurl = rawurl + "?" + paramStr
		} else {
			rawurl = rawurl + "&" + paramStr
		}
	}

	req.SetRequestURI(rawurl)

	var resp *fasthttp.Response = nil
	resp = fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	var err error
	if timeout > 0 {
		err = HttpClient.DoTimeout(req, resp, time.Duration(timeout))
	} else {
		err = HttpClient.Do(req, resp)
	}
	if err != nil {
		return nil, err
	}
	fmt.Println(resp.Header)
	code := resp.StatusCode()
	if code != http.StatusOK {
		return nil, errors.New(strconv.Itoa(code))
	}
	return resp.SwapBody(nil), nil
}

func TestHttpServer(t *testing.T) {
	rsp, err := HttpDo(
		"GET",
		"http://127.0.0.1:8090/healthz",
		Header{"test1": "test2"},
	)
	fmt.Println(rsp)
	fmt.Println(err)
}
