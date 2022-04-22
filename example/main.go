package main

import (
	"log"
	"os"

	"github.com/soapy/pbxproj/pbxproj"
)

func main() {
	projectPath := "project.pbxproj"
	project := pbxproj.NewPbxProject(projectPath)
	err := project.Parse()
	if err != nil {
		log.Fatal(err)
	}
	dumpToFile := func(name string) {
		// file, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY, 0644)
		// if err != nil {
		// 	log.Fatal(err)
		// }

		err = project.Dump(os.Stdout)
		if err != nil {
			log.Fatal(err)
		}
	}

	dumpToFile("OriginalProject.json")
	err = project.AddHeaderFile("foo.h")
	if err != nil {
		log.Fatal(err)
	}
	err = project.AddSourceFile("foo.m")
	if err != nil {
		log.Fatal(err)
	}
	err = project.AddFramework("FooKit.framework")
	if err != nil {
		log.Fatal(err)
	}

	dumpToFile("ModifiedProject.json")

	writer := pbxproj.NewPbxWriter(&project)
	writer.Write("ModifiedProject.pbxproj")
}
