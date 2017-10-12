package server

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type httpAPIFunc func(w http.ResponseWriter,
	r *http.Request,
	c *CniService,
	vars map[string]string,
) (interface{}, error)

// MakeHTTPHandler is a simple Wrapper for http handlers
func MakeHTTPHandler(c *CniService, handlerFunc httpAPIFunc) http.HandlerFunc {
	// Create a closure and return an anonymous function
	return func(w http.ResponseWriter, r *http.Request) {
		// Call the handler
		resp, err := handlerFunc(w, r, c, mux.Vars(r))
		if err != nil {
			// Log error
			log.Errorf("Handler for %s %s returned error: %s", r.Method, r.URL, err)

			if resp == nil {
				// Send HTTP response
				http.Error(w, err.Error(), http.StatusInternalServerError)
			} else {
				// Send HTTP response as Json
				content, err := json.Marshal(resp)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				w.WriteHeader(http.StatusInternalServerError)
				w.Write(content)
			}
		} else {
			// Send HTTP response as Json
			err = writeJSON(w, http.StatusOK, resp)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
}

// writeJSON: writes the value v to the http response stream as json with standard
// json encoding.
func writeJSON(w http.ResponseWriter, code int, v interface{}) error {
	// Set content type as json
	w.Header().Set("Content-Type", "application/json")
	// write the HTTP status code
	w.WriteHeader(code)
	// Write the Json output
	return json.NewEncoder(w).Encode(v)
}

// UnknownAction is a catchall handler for additional driver functions
func UnknownAction(w http.ResponseWriter, r *http.Request) {
	log.Infof("Unknown action at %q", r.URL.Path)
	content, _ := ioutil.ReadAll(r.Body)
	log.Infof("Body content: %s", string(content))
	http.NotFound(w, r)
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
