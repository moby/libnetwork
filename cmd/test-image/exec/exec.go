package exec

import (
	"fmt"
	"net/http"
	"net/url"
	osExec "os/exec"
	"strings"

	"github.com/docker/libnetwork/cmd/test-image/server"
	"github.com/sirupsen/logrus"
)

var basePaths = map[string]server.HTTPHandlerFunc{
	"/exec": exec,
}

func Init(s *server.TestServer) {
	s.RegisterHandler(basePaths)
}

func exec(s *server.TestServer, w http.ResponseWriter, r *http.Request) {
	logrus.Info("exec")
	r.ParseForm()

	logrus.Infof("form: %v", r.Form)
	// toPingList := strings.Split(r.Form, sep)

	for _, v := range r.Form {
		cmdDecoded, err := url.QueryUnescape(v[0])
		if err != nil {
			logrus.WithError(err).WithField("cmd", v).Error("unable to decode")
			return
		}
		cmdSplit := strings.Split(cmdDecoded, " ")
		cmd := osExec.CommandContext(r.Context(), cmdSplit[0], cmdSplit[1:]...)
		stdoutStderr, err := cmd.CombinedOutput()
		fmt.Fprint(w, string(stdoutStderr))
		if err != nil {
			logrus.WithError(err).WithField("cmd", cmdDecoded).WithField("stdouterr", string(stdoutStderr)).Error("exec command error")
			w.WriteHeader(400)
			return
		}
	}

	w.WriteHeader(200)
}
