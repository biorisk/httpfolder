httpfolder
==========

A quick and easy password protected web server for your files. httpfolder makes downloading/uploading files from your current working directory easy, even for fairly large files.

Download precompiled binary for linux/mac/windows: <a href="https://github.com/biorisk/httpfolder/releases/tag/v0.1">here</a>.

Usage:
httpfolder username password

options:
-p specify a port, default is 8080

example:
httpfolder -p 8080 biorisk notmyrealpassword

Check to make sure httpfolder is running by typing localhost:8080 (or the port you chose) into your browser. To access from other computers, use the IP address of your machine plus the port. e.g. 191.168.1.24:8080. Use Ctrl-C to stop.

Currently, only supports serving the current working directory. Will allow you to traverse directories below

Based on the FileServer in the standard golang http library.

If you wish to change the html upload form, you will need <a href="https://github.com/jteeuwen/go-bindata">go-bindata</a> installed.
