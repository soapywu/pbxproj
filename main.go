package main

import (
	"fmt"
)

func main() {
	aaa := []byte{'"', '\u003c', 'g', 'r', 'o', 'u', 'p', '\u003e', '"'}
	str := string(aaa)
	// str := pegparser.CharsToString(aaa)
	fmt.Println(str)

	// projectPath := "project.pbxproj"
	// data, err := ioutil.ReadFile(projectPath)
	// got, err := pegparser.ParseReader("", bytes.NewReader(data))
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// bytes, err := json.MarshalIndent(got, "", "  ")
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// fmt.Println(string(bytes))

}
