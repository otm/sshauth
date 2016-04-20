package main

import (
	"io"
	"io/ioutil"
	"os"
	"strings"
)

// Reads all .txt files in the current folder
// and encodes them as strings literals in textfiles.go
func main() {
	fs, _ := ioutil.ReadDir("assets")
	out, _ := os.Create("assets-generated.go")
	out.Write([]byte("package main \n\nconst (\n"))
	for _, f := range fs {
		if strings.HasSuffix(f.Name(), ".sh") {
			name := strings.TrimSuffix(f.Name(), ".sh")
			out.Write([]byte(name + " = `"))
			f, _ := os.Open("assets/" + f.Name())
			io.Copy(out, f)
			out.Write([]byte("`\n"))
		}
	}
	out.Write([]byte(")\n"))
}
