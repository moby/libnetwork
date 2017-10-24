package server

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"

	"github.com/docker/libnetwork/api"
	"github.com/docker/libnetwork/client"
	"github.com/docker/libnetwork/provider/cni"
)

type httpAPIFunc func(w http.ResponseWriter,
	r *http.Request,
	c cni.Service,
	vars map[string]string,
) (interface{}, error)

// MakeHTTPHandler is a simple Wrapper for http handlers
func MakeHTTPHandler(c cni.Service, handlerFunc httpAPIFunc) http.HandlerFunc {
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

func writeJSON(w http.ResponseWriter, code int, v interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	return json.NewEncoder(w).Encode(v)
}

// CopySandboxMetadata creates a sandbox metadata
func copySandboxMetadata(sbConfig client.SandboxCreate, externalKey string) api.SandboxMetadata {
	var meta api.SandboxMetadata
	meta.ContainerID = sbConfig.ContainerID
	meta.HostName = sbConfig.HostName
	meta.DomainName = sbConfig.DomainName
	meta.HostsPath = sbConfig.HostsPath
	meta.ResolvConfPath = sbConfig.ResolvConfPath
	meta.DNS = sbConfig.DNS
	meta.UseExternalKey = sbConfig.UseExternalKey
	meta.UseDefaultSandbox = sbConfig.UseDefaultSandbox
	meta.ExposedPorts = sbConfig.ExposedPorts
	meta.PortMapping = sbConfig.PortMapping
	meta.ExternalKey = externalKey
	//TODO: skipped extrahosts
	return meta
}

func getNetworkType(networkType string) string {
	switch networkType {
	case "dnet-overlay-net":
		return "overlay"
	case "dnet-bridge-net":
		return "bridge"
	case "dnet-ipvlan-net":
		return "ipvlan"
	case "dnet-macvlan-net":
		return "macvlan"
	default:
		return "overlay"
	}
}
