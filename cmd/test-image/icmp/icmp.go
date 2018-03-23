package icmp

import (
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/docker/libnetwork/cmd/test-image/server"
	"github.com/sirupsen/logrus"
)

var basePaths = map[string]server.HTTPHandlerFunc{
	"/icmp_ping": icmp_ping,
}

func Init(s *server.TestServer) {
	s.RegisterHandler(basePaths)
}

func icmp_ping(s *server.TestServer, w http.ResponseWriter, r *http.Request) {
	logrus.Info("ping")
	r.ParseForm()

	logrus.Infof("form: %v", r.Form)
	// toPingList := strings.Split(r.Form, sep)

	for _, v := range r.Form {
		for _, target := range strings.Split(v[0], ",") {
			cmd := exec.CommandContext(r.Context(), "ping", "-c", "1", target)
			stdoutStderr, err := cmd.CombinedOutput()
			if err != nil {
				logrus.WithError(err).WithField("cmd", fmt.Sprintf("ping -c 1 %s", target)).WithField("stdouterr", string(stdoutStderr)).Error("ping failed")
				w.WriteHeader(400)
				return
			}
		}
	}

	w.WriteHeader(200)
}
