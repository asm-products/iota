package endpointmgr

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"net/http"
	"net/rpc"
	"net/url"
	"os"
	"os/exec"
)

const PORT_START = 9000

type Endpoint struct {
	User          string
	Package       string
	Name          string
	parameterName string
}

type endpointProcess struct {
	Port     int
	Server   string //not used; everything localhost
	PID      int
	Endpoint *Endpoint
}

type EndpointMgr struct {
	EndpointRoot string
	epmap        map[string]endpointProcess
	updatech     chan Endpoint
	callch       chan *callParams
	nextport     int
}

type callParams struct {
	Endpoint   *Endpoint
	FormValues *url.Values
	Returnch   chan returnValues
	Parameter  string
}

type returnValues struct {
	S   *string
	Err error
}

func NewEndpointMgr(root string) (epm *EndpointMgr) {
	epm = new(EndpointMgr)
	epm.EndpointRoot = root

	epm.epmap = make(map[string]endpointProcess)
	epm.nextport = PORT_START
	epm.updatech = make(chan Endpoint)
	epm.callch = make(chan *callParams)
	go epm.serve()

	return
}

func (epm *EndpointMgr) GetEndpointFromSrc(src string, user string) (ep Endpoint, err error) {
	ep.User = user
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "foo.go", src, 0)
	if err != nil {
		return
	}
	ep.Package = f.Name.Name

	// find the first exported function that matches (string) (error) or (string) (string, error) signature
	for _, d := range f.Decls {
		switch x := d.(type) {
		case *ast.FuncDecl:
			// ast.Print(fset, x)
			plist := x.Type.Params.List
			if !x.Name.IsExported() || len(plist) != 1 || x.Type.Results == nil || len(x.Type.Results.List) != 2 {
				log.Println(x.Name.Name, "is not a valid function (wrong basic properties)")
				continue
			}

			ident, ok := plist[0].Type.(*ast.Ident)
			if !ok || ident.Name != "string" {
				log.Println(x.Name.Name, "is not a valid function (parameter is not a string)")
				continue
			}

			parameterName := plist[0].Names[0].Name

			ident, ok = x.Type.Results.List[0].Type.(*ast.Ident)
			if !ok || ident.Name != "string" {
				log.Println(x.Name.Name, "is not a valid function (first return type is not string)")
				continue
			}

			ident, ok = x.Type.Results.List[1].Type.(*ast.Ident)
			if !ok || ident.Name != "error" {
				log.Println(x.Name.Name, "is not a valid function (second return type is not error)")
				continue
			}

			// Found what we are looking for...
			ep.Name = x.Name.Name
			ep.parameterName = parameterName
			return ep, nil
		} // end case
	}
	err = errors.New("Unable to find an exported function with signature (string) (string, error)")
	return
}

// Notify the endpointmanager that the endpoint has changed and should be refreshed / restarted.
func (epm *EndpointMgr) Update(ep Endpoint) {
	epm.updatech <- ep
	log.Println("Sent channel update")
}

func (epm *EndpointMgr) Call(ep Endpoint, req *http.Request) (response string, err error) {
	rvChan := make(chan returnValues)
	err = req.ParseForm()
	if err != nil {
		return
	}
	cp := &callParams{
		Endpoint:   &ep,
		FormValues: &req.Form,
		Returnch:   rvChan,
	}

	epm.callch <- cp
	rv := <-cp.Returnch
	if rv.Err != nil {
		return "", rv.Err
	}
	return *rv.S, rv.Err
}

// Consume channel data & take appropriate action
func (epm *EndpointMgr) serve() {
	for {
		select {
		// Update channel
		case ep := <-epm.updatech:
			log.Println("Got an update request from the channel.")
			log.Printf("(Re)starting endpoint %v\n", ep)
			epp, ok := epm.epmap[(&ep).mapKey()]
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
			proc, ok := epm.epmap[endpoint.mapKey()]
			if !ok {
				msg := fmt.Sprintf("No process found for endpoint %v", endpoint)
				log.Println(msg)
				params.Returnch <- returnValues{Err: errors.New(msg)}
				continue
			}
			// only proc.Endpoint has parameterName; the one passed in from params doesn't know it.
			paramName := proc.Endpoint.parameterName
			log.Println("parsing form values", params.FormValues)
			paramValues, ok := (*params.FormValues)[paramName]
			if !ok {
				params.Returnch <- returnValues{Err: errors.New("Parameter '" + paramName + "' not found")}
				return
			} else if len(paramValues) != 1 {
				params.Returnch <- returnValues{Err: errors.New(fmt.Sprintf(
					"Multiple values for parameter '%s'", paramName))}
				return
			}
			params.Parameter = paramValues[0]
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
		epp := endpointProcess{Port: port,
			Server:   "localhost",
			PID:      cmd.Process.Pid,
			Endpoint: ep,
		}
		epm.epmap[ep.mapKey()] = epp
		log.Println("Success spawning new process, PID", cmd.Process.Pid, "Port", port)
	}
	return
}

func (ep *Endpoint) mapKey() string {
	return fmt.Sprintf("%s.%s.%s", ep.User, ep.Package, ep.Name)
}

func callrpc(params *callParams, proc endpointProcess) {
	tmp := ""
	rv := returnValues{S: &tmp, Err: nil}
	client, err := rpc.DialHTTP("tcp", fmt.Sprintf("%s:%d", proc.Server, proc.Port))
	if err != nil {
		log.Println("Error Dialing remote", *params.Endpoint, proc, err)
		rv.Err = err
		params.Returnch <- rv
		return
	}
	log.Println("Making RPC", params.Parameter)
	rv.Err = client.Call("Iotasvc.ServeHttp", params.Parameter, rv.S)
	params.Returnch <- rv
}
