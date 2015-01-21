package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/asm-products/iota/endpointmgr"
	"github.com/gorilla/mux"
	"go/ast"
	"go/parser"
	"go/token"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	txttemplate "text/template"
)

const ENDPOINT_ROOT = "endpoints"

type srcTemplateData struct {
	User     string
	Package  string
	Filename string
	Src      string
	ErrMsg   string
}

type BuildData struct {
	UserDir      string
	Endpoint     *endpointmgr.Endpoint
	Src          *string
	Endpointmain string
}

func endpointHandler(w http.ResponseWriter, r *http.Request, epm *endpointmgr.EndpointMgr) {
	vars := mux.Vars(r)
	ep := endpointmgr.Endpoint{
		User:    vars["user"],
		Package: vars["package"],
		Name:    vars["fname"],
	}
	var epResponse *string
	err := epm.Call(ep, r, epResponse)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
	} else {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, *epResponse)
	}
}

func srcHandler(w http.ResponseWriter, r *http.Request, epm *endpointmgr.EndpointMgr) {
	vars := mux.Vars(r)

	// Would probably cache, but easier to work with a live file for now
	t, err := template.ParseFiles("templates/src.html")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "[500] Error parsing template\n%s", err)
		return
	}

	std := &srcTemplateData{
		User:     vars["user"],
		Filename: vars["filename"],
		Package:  vars["package"],
	}
	userDir := fmt.Sprintf("%s/%s/", ENDPOINT_ROOT, std.User) // FIXME: make sure User is safe
	userDir, err = filepath.Abs(userDir)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Error finding user directory: ", err)
	}
	srcDir := fmt.Sprintf("%s/src/%s/", userDir, std.Package)
	dir := http.Dir(srcDir)
	if r.Method == "GET" {
		f, err := dir.Open(std.Filename)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			std.ErrMsg = err.Error()
		} else {
			b := new(bytes.Buffer)
			_, err = io.Copy(b, f)
			if err != nil {
				std.ErrMsg = "Error reading file, err: " + err.Error()
			} else {
				std.Src = b.String()
			}
			f.Close()
		}
		err = t.Execute(w, std)
		if err != nil {
			fmt.Printf("Template error: %s\n", err)
		}
	} else if r.Method == "POST" {
		srcFilename := srcDir + std.Filename
		status, src, err := saveSrcPost(r, srcFilename)
		w.WriteHeader(status)
		if err != nil {
			fmt.Fprintf(w, "Error: %s", err)
			return
		}
		msg := fmt.Sprintf("Source file %s saved.\n\n", std.Filename)
		// Build src
		ep, err := doBuild(src, std.Package, userDir)
		if err != nil {
			fmt.Fprintf(w, "%sBuild Errors:\n%s\n\nSource:\n%s", msg, err, src)
			return
		}
		ep.User = vars["user"]
		fmt.Fprintf(w, "%sBuild Success!\n%s\n", msg, src)
		epm.Update(ep)
		fmt.Fprint(w, "Service Started.")
	}
}

func saveSrcPost(r *http.Request, filename string) (status int, src string, err error) {
	src = r.FormValue("src")
	if src == "" {
		return http.StatusBadRequest, src, errors.New("No src data received")
	}
	f, err := os.Create(filename) // FIXME: better validation needs to happen
	if err != nil {
		return http.StatusInternalServerError, src, err
	}
	defer f.Close()
	_, err = f.Write([]byte(src))
	if err != nil {
		return http.StatusInternalServerError, src, err
	}
	return http.StatusOK, src, err
}

func doBuild(src string, packageNameURL string, userDir string) (ep endpointmgr.Endpoint, err error) {
	ep, err = parseSrc(src)
	if err != nil {
		return
	}
	bd := &BuildData{
		Endpoint: &ep,
		Src:      &src,
		UserDir:  userDir,
	}
	if ep.Package != packageNameURL {
		msg := fmt.Sprintf("Source package name '%s' does not match URL packages name '%s'", ep.Package, packageNameURL)
		return ep, errors.New(msg)
	}
	fmt.Println("Package:", ep.Package, "Name:", ep.Name)

	err = bd.renderEndpointMain()
	if err != nil {
		return
	}
	err = bd.build()
	return
}

func parseSrc(src string) (ep endpointmgr.Endpoint, err error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "foo.go", src, 0)
	if err != nil {
		return
	}
	ep.Package = f.Name.Name

	// find the first exported function that matches the (*http.Request, *string) (error) signature
	// Can't rely on 'http' (user could redifne) so just rely on the actual fn name for now
	for _, d := range f.Decls {
		switch x := d.(type) {
		case *ast.FuncDecl:
			ast.Print(fset, x)
			plist := x.Type.Params.List
			if !x.Name.IsExported() || len(plist) != 2 || x.Type.Results == nil || len(x.Type.Results.List) != 1 {
				fmt.Println(x.Name.Name, "is not a valid function (wrong basic properties)")
				continue
			}

			// both params are *ast.StarExpr types, so we can do in a loop
			expectNames := [2]string{"Request", "string"}
			paramsOk := false
			for i, expect := range expectNames {
				ps, psok := plist[i].Type.(*ast.StarExpr)
				if psok {
					var name string
					if i == 0 {
						var se *ast.SelectorExpr
						se, paramsOk = ps.X.(*ast.SelectorExpr)
						if paramsOk {
							name = se.Sel.Name
						}
					} else {
						var ident *ast.Ident
						ident, paramsOk = ps.X.(*ast.Ident)
						if paramsOk {
							name = ident.Name
						}
					}
					paramsOk = paramsOk && (name == expect)
				} else {
					paramsOk = false
				}
				if !paramsOk {
					fmt.Println(x.Name.Name, "is not a valid function, parameter", i, "is not *", expect)
					break
				}
			}

			returnOk := false
			if paramsOk {
				// check for error return
				result, ok := x.Type.Results.List[0].Type.(*ast.Ident)
				if ok && result.Name == "error" {
					returnOk = true
				} else {
					fmt.Println(x.Name.Name, "does not return error")
				}
			}

			if paramsOk && returnOk {
				ep.Name = x.Name.Name
				return ep, nil
			}
		} // end case
	}
	err = errors.New("Unable to find an exported function with signature (*http.Request, *string) error")
	return
}

func (bd *BuildData) renderEndpointMain() (err error) {
	tmpl, err := txttemplate.ParseFiles("templates/endpointmain.go")
	if err != nil {
		return
	}
	var b bytes.Buffer
	err = tmpl.Execute(&b, bd.Endpoint)
	if err != nil {
		return
	}
	bd.Endpointmain = b.String()
	return
}

func (bd *BuildData) build() (err error) {
	f, err := ioutil.TempFile("", "endpointmain")
	if err != nil {
		return
	}
	fName := f.Name()
	err = func() (e error) {
		defer f.Close()
		_, err = f.Write([]byte(bd.Endpointmain))
		return
	}()
	if err != nil {
		return
	}
	var buildOut bytes.Buffer
	var buildErr bytes.Buffer
	buildCmd := exec.Command("./buildendpoint.sh", bd.UserDir, bd.Endpoint.Package, fName)
	buildCmd.Stderr = &buildErr
	buildCmd.Stdout = &buildOut
	err = buildCmd.Run()
	fmt.Println("Out:", buildOut.String())
	fmt.Println("Err:", buildErr.String())

	return
}

func main() {
	epm := endpointmgr.NewEndpointMgr(ENDPOINT_ROOT)
	r := mux.NewRouter()
	r.HandleFunc("/{user}/{package}/{filename}/src",
		func(w http.ResponseWriter, req *http.Request) {
			srcHandler(w, req, epm)
		})
	r.HandleFunc("/{user}/{package}/f/{fname}",
		func(w http.ResponseWriter, req *http.Request) {
			endpointHandler(w, req, epm)
		})
	http.Handle("/", r)
	http.ListenAndServe(":8000", nil)
}
