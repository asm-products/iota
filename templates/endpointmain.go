package main

import (
	"flag"
	"net/http"
	"net/rpc"
	"os"
	"strconv"
	"{{ .Package }}"
)

var userf = {{.Package}}.{{.Name}}

type Iotasvc struct{}

func (isvc *Iotasvc) ServeHttp(in string, out *string) (err error) {
	*out, err = userf(in)
	return err
}

func getPort() (port int) {
	flag.IntVar(&port, "port", 0, "Port the service will listen on [required]")
	flag.Parse()
	if port == 0 {
		flag.Usage()
		os.Exit(2)
	}
	return
}

func main() {
	port := getPort()
	isvc := &Iotasvc{}
	err := rpc.Register(isvc)
	if err != nil {
		panic(err)
	}
	rpc.HandleHTTP()

	addr :=  ":" + strconv.Itoa(port)
	err = http.ListenAndServe(addr, nil)
	if err != nil {
		panic(err)
	}
}
