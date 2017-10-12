package netutils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
)

// HTTPConnection holds the protocol and address info for http connection
type HTTPConnection struct {
	// Proto holds the client protocol i.e. unix.
	Proto string
	// Addr holds the client address.
	Addr string
}

// HTTPCall is used to make a http call using httpconnection information
func (h *HTTPConnection) HTTPCall(method, path string, data interface{}, headers map[string][]string) (io.ReadCloser, http.Header, int, error) {
	var in io.Reader
	in, err := encodeData(data)
	if err != nil {
		return nil, nil, -1, err
	}

	req, err := http.NewRequest(method, fmt.Sprintf("%s", path), in)
	if err != nil {
		return nil, nil, -1, err
	}

	setupRequestHeaders(method, data, req, headers)

	req.URL.Host = h.Addr
	req.URL.Scheme = "http"
	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	statusCode := -1
	if resp != nil {
		statusCode = resp.StatusCode
	}
	if err != nil {
		return nil, nil, statusCode, fmt.Errorf("error when trying to connect: %v", err)
	}

	if statusCode < 200 || statusCode >= 400 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, nil, statusCode, err
		}
		return nil, nil, statusCode, fmt.Errorf("error : %s", bytes.TrimSpace(body))
	}

	return resp.Body, resp.Header, statusCode, nil
}

func setupRequestHeaders(method string, data interface{}, req *http.Request, headers map[string][]string) {
	if data != nil {
		if headers == nil {
			headers = make(map[string][]string)
		}
		headers["Content-Type"] = []string{"application/json"}
	}

	expectedPayload := (method == "POST" || method == "PUT")

	if expectedPayload && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "text/plain")
	}

	if headers != nil {
		for k, v := range headers {
			req.Header[k] = v
		}
	}
}

func encodeData(data interface{}) (*bytes.Buffer, error) {
	params := bytes.NewBuffer(nil)
	if data != nil {
		if err := json.NewEncoder(params).Encode(data); err != nil {
			return nil, err
		}
	}
	return params, nil
}

// ReadBody reads http body from stream
func ReadBody(stream io.ReadCloser, hdr http.Header, statusCode int, err error) ([]byte, int, error) {
	if stream != nil {
		defer stream.Close()
	}
	if err != nil {
		return nil, statusCode, err
	}
	body, err := ioutil.ReadAll(stream)
	if err != nil {
		return nil, -1, err
	}
	return body, statusCode, nil
}
