package endpointmgr

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/rpc"
	"os"
	"os/exec"
)

const PORT_START = 9000

type Endpoint struct {
	User    string
	Package string
	Name    string
}

type endpointProcess struct {
	Port   int
	Server string //not used; everything localhost
	PID    int
}

type EndpointMgr struct {
	EndpointRoot string
	epmap        map[Endpoint]endpointProcess
	updatech     chan Endpoint
	callch       chan *callParams
	nextport     int
}

type callParams struct {
	Endpoint *Endpoint
	Req      *http.Request
	Returnch chan returnValues
}

type returnValues struct {
	S   *string
	Err error
}

func NewEndpointMgr(root string) (epm *EndpointMgr) {
	epm = new(EndpointMgr)
	epm.EndpointRoot = root

	epm.epmap = make(map[Endpoint]endpointProcess)
	epm.nextport = PORT_START
	epm.updatech = make(chan Endpoint)
	epm.callch = make(chan *callParams)
	go epm.serve()

	return
}

// Notify the endpointmanager that the endpoint has changed and should be refreshed / restarted.
func (epm *EndpointMgr) Update(ep Endpoint) {
	log.Println("Sending a channel update")
	epm.updatech <- ep
	log.Println("Sent channel update")
}

func (epm *EndpointMgr) Call(ep Endpoint, req *http.Request, s *string) (err error) {
	rvChan := make(chan returnValues)
	cp := &callParams{
		Endpoint: &ep,
		Req:      req,
		Returnch: rvChan,
	}

	epm.callch <- cp
	rv := <-cp.Returnch
	s = rv.S
	return rv.Err
}

// Consume channel data & take appropriate action
func (epm *EndpointMgr) serve() {
	for {
		select {
		// Update channel
		case ep := <-epm.updatech:
			log.Println("Got an update request from the channel.")
			log.Printf("(Re)starting endpoint %v\n", ep)
			epp, ok := epm.epmap[ep]
			port := 0
			if ok {
				// kill the existing process
				p, err := os.FindProcess(epp.PID)
				if err != nil {
					log.Printf("os.FindProcess(%d) error: %s\n", epp.PID, err)
				} else {
					err = p.Kill()
					if err != nil {
						log.Printf("error killing PID %d, error: %s\n", epp.PID, err)
					} else {
						// reuse the port
						port = epp.Port
					}
				}
			}
			// start a new process
			if port == 0 {
				port = epm.nextport
				epm.nextport++
			}
			err := epm.runEndpoint(&ep, port)
			if err != nil {
				log.Println("Error in runEndpoint,", err)
			}
		case params := <-epm.callch:
			log.Println("Got a request for a endpoint function")
			endpoint := *params.Endpoint
			proc, ok := epm.epmap[endpoint]
			if !ok {
				msg := fmt.Sprintf("No process found for endpoint %v", endpoint)
				log.Println(msg)
				params.Returnch <- returnValues{Err: errors.New(msg)}
				continue
			}
			go callrpc(params, proc)
		}
	}
}

// Get the the filename for the requested endpoint
func (epm *EndpointMgr) getEndpointFilename(ep *Endpoint) (epFilename string) {
	return fmt.Sprintf("%s/%s/f/%s/endpoint", epm.EndpointRoot, ep.User, ep.Package)
}

func (epm *EndpointMgr) runEndpoint(ep *Endpoint, port int) (err error) {
	epFilename := epm.getEndpointFilename(ep)
	cmd := exec.Command(epFilename, "--port", fmt.Sprintf("%d", port))
	err = cmd.Start()
	if err == nil {
		epp := endpointProcess{Port: port, Server: "localhost", PID: cmd.Process.Pid}
		epm.epmap[*ep] = epp
		log.Println("Success spawning new process, PID", cmd.Process.Pid, "Port", port)
	}
	return
}

func callrpc(params *callParams, proc endpointProcess) {
	rv := returnValues{S: nil, Err: nil}
	client, err := rpc.DialHTTP("tcp", fmt.Sprintf("%s:%d", proc.Server, proc.Port))
	if err != nil {
		log.Println("Error Dialing remote", *params.Endpoint, proc, err)
		rv.Err = err
		params.Returnch <- rv
		return
	}
	rv.Err = client.Call("Iotasvc.ServeHttp", params.Req, rv.S)
	params.Returnch <- rv
}
