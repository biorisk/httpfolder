package main

import (
	"flag"
	"fmt"
	"github.com/abbot/go-http-auth"
	"log"
	"net/http"
	"os"
)

var port string
var usr string
var passwd string

func main() {
	flag.StringVar(&port, "p", "8080", "port to listen on")
	flag.Parse()
	usr = flag.Arg(0)
	passwd = flag.Arg(1)
	cwd, err := os.Getwd()
	fmt.Printf("Serving: %s on port %s\n", cwd, port)
	if err != nil {
		log.Fatal(err)
	}

	authenticator := auth.NewBasicAuthenticator("Please login.", Secret)
	http.HandleFunc("/",
		authenticator.Wrap(func(res http.ResponseWriter, req *auth.AuthenticatedRequest) {
			FileServer(Dir(cwd)).ServeHTTP(res, &req.Request)
		}))
	
	http.ListenAndServe((":"+port), nil)
}

func Secret(user, realm string) string {
	mymd5 := ""
	if user == usr {
		mymd5 = string(auth.MD5Crypt([]byte(passwd), []byte("mymysalt"), []byte("$apr1$")))
	}
	return mymd5
}