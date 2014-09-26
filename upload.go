package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

var chttp = http.NewServeMux()

func main() {

	chttp.Handle("/upload/", http.StripPrefix("/upload/", http.FileServer(http.Dir("./up"))))

	http.HandleFunc("/", homeHandle)

	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func homeHandle(w http.ResponseWriter, req *http.Request) {
	fmt.Println(req.URL.Path)
	switch {
	case strings.Contains(req.URL.Path, "/formupload"):
		receiveUpload(w, req)
	case strings.Contains(req.URL.Path, "/up"):
		chttp.ServeHTTP(w, req)
	case strings.Contains(req.URL.Path, "/dataurl"):
		receiveDataURL(w, req)
	default:
		http.Error(w, http.StatusText(404), 404)
	}
}

// code to receive posted forms with large file uploads by streaming to disk then parsing.
func receiveUpload(w http.ResponseWriter, req *http.Request) {
	t := time.Now()
	dir := t.Format("20060102150405")
	fmt.Printf("time: %s\n", dir)

	reader, err := req.MultipartReader()
	if err != nil {
		fmt.Println(err)
		http.Error(w, "not a form", http.StatusBadRequest)
	}
	form, err := reader.ReadForm(100000)
	defer form.RemoveAll()
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println("incoming form")
	formFile, err := os.Create("./uploads/" + dir + "_README")
	if err != nil {
		panic(err)
	}
	defer formFile.Close()
	for k, v := range form.Value {
		fmt.Printf("key: %s  value: %s\n", k, v[0])
		// 		fmt.Fprintf(formFile,"key: %s  value: %s\n", k, v[0])
	}
	form.Value["time"] = []string{dir}
	formJson, err := json.MarshalIndent(form.Value, "", "\t")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Fprintln(formFile, string(formJson))
	formFile.Close()
	if len(form.File) > 0 { //check to make sure form has attached files before creating directory
		fmt.Printf("creating directory: ./uploads/%s\n", dir)
		os.Mkdir(fmt.Sprintf("./uploads/%s", dir), 0777)
	}
	fmt.Fprint(w, "<html><body><h2>Uploaded</h2>\n")
	for k, files := range form.File { //loop through each file selector in submitted form
		for i := range files { //loop through each file from current file selector
			fmt.Printf("key: %s  value: %s\n", k, files[i].Filename)
			srcfile, err := files[i].Open()
			if err != nil {
				fmt.Println(err)
			}
			dstfile, err := os.Create(fmt.Sprintf("./uploads/%s/%s", dir, files[i].Filename))
			if err != nil {
				fmt.Println(err)
			}
			sizecopied, err := io.Copy(dstfile, srcfile)
			if err != nil {
				fmt.Println(err)
			}
			fmt.Printf("file size: %d\n", sizecopied)
			fmt.Fprintf(w, "<b>%s</b>: %d bytes<br>\n", files[i].Filename, sizecopied)
			srcfile.Close()
			dstfile.Close()
		}
	}
	fmt.Fprint(w, "<p><h2>Done uploading files.</h2></body></html>")
}

func receiveDataURL(w http.ResponseWriter, req *http.Request) {
	t := time.Now()
	dir := t.Format("20060102150405")
	fmt.Printf("time: %s\n", dir)

	err := req.ParseForm()
	if err != nil {
		fmt.Println(err)
		http.Error(w, "not a form", http.StatusBadRequest)
	}
	req.Form["time"] = []string{dir}
	urlJson, err := json.MarshalIndent(req.Form, "", "\t")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(string(urlJson))
	urlFile, err := os.OpenFile("./dataurl/dataurl.txt", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660)
	if err != nil {
		panic(err)
	}
	defer urlFile.Close()
	fmt.Fprintln(urlFile, string(urlJson))
	urlFile.Close()
	firstName, uri, title := req.FormValue("fname"), req.FormValue("uri"), req.FormValue("title")
	if firstName == "" || uri == "" {
		fmt.Fprintln(w, "<html>"+
			"<body>Improperly formatted submission to GeneStation, please create new bookmarklet <a href='"+"http://genestation.org/upload/bookmarklet.html'>here</a>"+
			"</body></html>")
	} else {
		fmt.Fprintf(w, "<html><body>%s, thank you for submitting:" +
			"<br>%s<br>%s</body></html>\n", firstName, title, uri)
	}
}
