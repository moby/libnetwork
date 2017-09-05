package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"

	"github.com/docker/docker/pkg/reexec"
	"github.com/docker/docker/pkg/term"
	"github.com/docker/libnetwork/pkg/cniapi"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

const (
	CNIServerPort = 9005
)

type CniService struct {
	//k8sClient *APIClient
	sandboxIDStore  map[string]string // containerID to sandboxID mapping
	endpointIDStore map[string]string // containerID to endpointID mapping
}

var cniService *CniService

// InitCNIServer initializes the cni server
func InitCNIServer(serverCloseChan chan struct{}) error {

	log.Infof("Starting CNI server")
	cniService = newCniService()

	router := mux.NewRouter()
	t := router.Headers("Content-Type", "application/json").Methods("POST").Subrouter()
	t.HandleFunc(cniapi.AddPodUrl, MakeHTTPHandler(addPod))
	t.HandleFunc(cniapi.DelPodUrl, MakeHTTPHandler(deletePod))

	driverPath := cniapi.LibnetworkCNISock
	os.Remove(driverPath)
	os.MkdirAll(cniapi.PluginPath, 0700)
	go func() {
		l, err := net.ListenUnix("unix", &net.UnixAddr{Name: driverPath, Net: "unix"})
		if err != nil {
			panic(err)
		}
		log.Infof("Libnetwork CNI plugin listening on on %s", driverPath)
		http.Serve(l, router)
		l.Close()
		close(serverCloseChan)
	}()
	return nil
}

type httpAPIFunc func(w http.ResponseWriter, r *http.Request, vars map[string]string) (interface{}, error)

// MakeHTTPHandler is a simple Wrapper for http handlers
func MakeHTTPHandler(handlerFunc httpAPIFunc) http.HandlerFunc {
	// Create a closure and return an anonymous function
	return func(w http.ResponseWriter, r *http.Request) {
		// Call the handler
		resp, err := handlerFunc(w, r, mux.Vars(r))
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

func httpCall(method, path string, data interface{}, headers map[string][]string) (io.ReadCloser, http.Header, int, error) {
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

	req.URL.Host = fmt.Sprintf("0.0.0.0:%d", 2385)
	req.URL.Scheme = "http"
	fmt.Printf("Requesting http: %+v", req)
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

func readBody(stream io.ReadCloser, hdr http.Header, statusCode int, err error) ([]byte, int, error) {
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

func newCniService() *CniService {
	c := new(CniService)
	c.sandboxIDStore = make(map[string]string)
	c.endpointIDStore = make(map[string]string)
	return c
}

func main() {
	fmt.Printf("Starting CNI server")
	if reexec.Init() {
		return
	}

	_, _, stderr := term.StdStreams()
	log.SetOutput(stderr)
	serverCloseChan := make(chan struct{})
	if err := InitCNIServer(serverCloseChan); err != nil {
		fmt.Printf("Failed to initialize CNI server: \n", err)
		os.Exit(1)
	}
	<-serverCloseChan
}
