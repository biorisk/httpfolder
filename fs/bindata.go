package httpfolder

import (
	"fmt"
	"io/ioutil"
	"strings"
)

// bindata_read reads the given file from disk. It returns an error on failure.
func bindata_read(path, name string) ([]byte, error) {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		err = fmt.Errorf("Error reading asset %s at %s: %v", name, path, err)
	}
	return buf, err
}

// html_form_html reads file data from disk. It returns an error on failure.
func html_form_html() ([]byte, error) {
	return bindata_read(
		"/home/kris/go/src/projects/httpfolder/fs/html/form.html",
		"html/form.html",
	)
}

// html_jquery_form_js reads file data from disk. It returns an error on failure.
func html_jquery_form_js() ([]byte, error) {
	return bindata_read(
		"/home/kris/go/src/projects/httpfolder/fs/html/jquery.form.js",
		"html/jquery.form.js",
	)
}

// html_jquery_js reads file data from disk. It returns an error on failure.
func html_jquery_js() ([]byte, error) {
	return bindata_read(
		"/home/kris/go/src/projects/httpfolder/fs/html/jquery.js",
		"html/jquery.js",
	)
}

// Asset loads and returns the asset for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func Asset(name string) ([]byte, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		return f()
	}
	return nil, fmt.Errorf("Asset %s not found", name)
}

// AssetNames returns the names of the assets.
func AssetNames() []string {
	names := make([]string, 0, len(_bindata))
	for name := range _bindata {
		names = append(names, name)
	}
	return names
}

// _bindata is a table, holding each asset generator, mapped to its name.
var _bindata = map[string]func() ([]byte, error){
	"html/form.html": html_form_html,
	"html/jquery.form.js": html_jquery_form_js,
	"html/jquery.js": html_jquery_js,
}
// AssetDir returns the file names below a certain
// directory embedded in the file by go-bindata.
// For example if you run go-bindata on data/... and data contains the
// following hierarchy:
//     data/
//       foo.txt
//       img/
//         a.png
//         b.png
// then AssetDir("data") would return []string{"foo.txt", "img"}
// AssetDir("data/img") would return []string{"a.png", "b.png"}
// AssetDir("foo.txt") and AssetDir("notexist") would return an error
// AssetDir("") will return []string{"data"}.
func AssetDir(name string) ([]string, error) {
	node := _bintree
	if len(name) != 0 {
		cannonicalName := strings.Replace(name, "\\", "/", -1)
		pathList := strings.Split(cannonicalName, "/")
		for _, p := range pathList {
			node = node.Children[p]
			if node == nil {
				return nil, fmt.Errorf("Asset %s not found", name)
			}
		}
	}
	if node.Func != nil {
		return nil, fmt.Errorf("Asset %s not found", name)
	}
	rv := make([]string, 0, len(node.Children))
	for name := range node.Children {
		rv = append(rv, name)
	}
	return rv, nil
}

type _bintree_t struct {
	Func func() ([]byte, error)
	Children map[string]*_bintree_t
}
var _bintree = &_bintree_t{nil, map[string]*_bintree_t{
	"html": &_bintree_t{nil, map[string]*_bintree_t{
		"form.html": &_bintree_t{html_form_html, map[string]*_bintree_t{
		}},
		"jquery.form.js": &_bintree_t{html_jquery_form_js, map[string]*_bintree_t{
		}},
		"jquery.js": &_bintree_t{html_jquery_js, map[string]*_bintree_t{
		}},
	}},
}}
