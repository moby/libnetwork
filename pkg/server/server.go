package main

import (
	"fmt"
	"os"
	
	log "github.com/sirupsen/logrus"
	"github.com/docker/docker/pkg/reexec"
        "github.com/docker/docker/pkg/term"
	cniserver "github.com/docker/libnetwork/pkg/server/cniserver"
)

func main(){
	fmt.Printf("Starting CNI server")
	        if reexec.Init() {
                return
        }

        _, _, stderr := term.StdStreams()
        log.SetOutput(stderr)
	serverCloseChan := make(chan struct{})
	if err:= cniserver.InitCNIServer(serverCloseChan);err !=nil{
		fmt.Printf("Failed to initialize CNI server: \n",err)
		os.Exit(1)
	}
	<-serverCloseChan
}
