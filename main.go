package main

import (
	"log"
	"os"

	"example.com/peg/pbxproj"
)

func main() {
	projectPath := "project.pbxproj"
	project := pbxproj.NewPbxProject(projectPath)
	err := project.Parse()
	if err != nil {
		log.Fatal(err)
	}
	dumpToFile := func(name string) {
		file, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatal(err)
		}

		err = project.Dump(file)
		if err != nil {
			log.Fatal(err)
		}
	}

	dumpToFile("before.json")
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

	dumpToFile("after.json")

	writer := pbxproj.NewPbxWriter(&project)
	writer.Write("projectAfter.pbxproj")
}
