//
// See the file COPYRIGHT for copyright information.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package main

import (
	"crypto/rand"
	"embed"
	_ "embed"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

var (
	filePath       = flag.String("filepath", "", "Absolute path to the file that this program will manage, e.g. /home/sean/myfile.pdf")
	secret         = flag.String("secret", rand.Text(), "Secret string a client must provide in the web page")
	port           = flag.Int("port", 0, "Port to listen on. Defaults to picking a random port.")
	maxRequestSize = flag.Int64("max-request-size", 100<<20, "Max request size in bytes")
)

//go:embed index.html
var htmlFile embed.FS

func main() {
	flag.Parse()
	if *filePath == "" {
		log.Fatal("--filepath is required")
	}
	log.Printf("Will write to file %v, using secret %v", *filePath, *secret)

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServerFS(htmlFile))

	// mut is used to only allow one caller to upload at a time
	var mut sync.Mutex

	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		mut.Lock()
		defer mut.Unlock()

		r.Body = http.MaxBytesReader(w, r.Body, *maxRequestSize)

		err := r.ParseMultipartForm(*maxRequestSize) // allow using a maximum of 100 MiB when parsing
		if err != nil {
			writeResponse(w, err.Error(), http.StatusInternalServerError)
			return
		}
		clientSecret := r.PostForm.Get("secret")
		if clientSecret != *secret {
			writeResponse(w, "incorrect secret", http.StatusUnauthorized)
			return
		}
		file, _, err := r.FormFile("pdf-upload")
		if err != nil {
			writeResponse(w, err.Error(), http.StatusInternalServerError)
			return
		}
		outFile, err := os.Create(*filePath)
		if err != nil {
			writeResponse(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer shut(outFile)
		_, err = io.Copy(outFile, file)
		if err != nil {
			writeResponse(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeResponse(w, "File uploaded successfully", http.StatusCreated)
	})
	server := &http.Server{
		Handler:        mux,
		ReadTimeout:    1 * time.Minute,
		WriteTimeout:   1 * time.Minute,
		MaxHeaderBytes: 1 << 20,
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(*port)))
	must(err)
	addr := net.JoinHostPort("", strconv.FormatInt(int64(listener.Addr().(*net.TCPAddr).Port), 10))
	log.Printf("Listening on %v", addr)
	must(server.Serve(listener))
}

func writeResponse(w http.ResponseWriter, errMsg string, code int) {
	http.Error(w, errMsg, code)
	log.Printf("Response %v: %v", code, errMsg)
}

// must logs an error and panics. This should only be done for
// startup errors, not after the server is up and running.
func must(err error) {
	if err != nil {
		panic("got a startup error: " + err.Error())
	}
}

func shut(c io.Closer) {
	_ = c.Close()
}
