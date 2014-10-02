package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/abbot/go-http-auth"
	"log"
	"net/http"
	"net"
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
	ip, err := localIP();
	fmt.Printf("Serving: %s at http://%s:%s\n", cwd, ip, port)
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

func localIP() (net.IP, error) { 
	tt, err := net.Interfaces() 
	if err != nil { 
			return nil, err 
	} 
	for _, t := range tt { 
			aa, err := t.Addrs() 
			if err != nil { 
					return nil, err 
			} 
			for _, a := range aa { 
					ipnet, ok := a.(*net.IPNet) 
					if !ok { 
							continue 
					} 
					v4 := ipnet.IP.To4() 
					if v4 == nil || v4[0] == 127 { // loopback address 
							continue 
					} 
					return v4, nil 
			} 
	}
	return nil, errors.New("cannot find local IP address") 
} 