package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"html/template"
	"io"
	"net/http"
	"os"
)

var ENDPOINT_ROOT = "endpoints"

type srcTemplateData struct {
	UserID   string
	Filename string
	Src      string
	ErrMsg   string
}

func srcHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	// Would probably cache, but easier to work with a live file for now
	t, err := template.ParseFiles("templates/src.html")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "[500] Error parsing template\n%s", err)
		return
	}

	std := &srcTemplateData{UserID: vars["userid"], Filename: vars["filename"]}
	src_dir := ENDPOINT_ROOT + "/src/" + std.UserID + "/" // FIXME: make sure UserID is safe
	dir := http.Dir(src_dir)
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
		status, src, err := saveSrcPost(r, src_dir+std.Filename)
		w.WriteHeader(status)
		if err != nil {
			fmt.Fprintf(w, "Error: %s", err)
		} else {
			fmt.Fprintf(w, "Source file %s saved.\n\n%s", std.Filename, src)
		}
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

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/src/{userid}/{filename}", srcHandler)
	http.Handle("/", r)
	http.ListenAndServe(":8000", nil)
}
