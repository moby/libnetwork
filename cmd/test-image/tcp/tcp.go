package tcp

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/docker/libnetwork/cmd/test-image/server"
	"github.com/sirupsen/logrus"
)

var basePaths = map[string]server.HTTPHandlerFunc{
	"/tcp_ping": tcp_ping,
	"/tcp_pong": tcp_pong,
}

func Init(s *server.TestServer) {
	s.RegisterHandler(basePaths)
}

func tcp_ping(s *server.TestServer, w http.ResponseWriter, r *http.Request) {
	logrus.Info("tcp_ping")
	r.ParseForm()

	logrus.Infof("form: %v", r.Form)
	// toPingList := strings.Split(r.Form, sep)

	for _, v := range r.Form {
		for _, target := range strings.Split(v[0], ",") {
			path := fmt.Sprintf("http://%s/tcp_pong", target)
			_, err := http.Get(path)
			if err != nil {
				logrus.WithError(err).WithField("cmd", fmt.Sprintf("GET %s", path)).Error("tcp_ping failed")
				w.WriteHeader(400)
				return
			}
		}
	}

	w.WriteHeader(200)
}

func tcp_pong(s *server.TestServer, w http.ResponseWriter, r *http.Request) {
	logrus.Info("tcp_pong")
	w.WriteHeader(200)
}
