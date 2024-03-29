# XcodeProj

Go implement of Parser utility for xcodeproj project files, Allows you to edit xcodeproject files and write them back out.

It's heavily mimicked and inspired by [cordova-node-xcode](https://github.com/apache/cordova-node-xcode).

*Warning*: this project was not fully tested, use it at yout own risk.

# Usage
```go
    package main

    import (
        "log"
        "github.com/soapywu/pbxproj/pbxproj"
    )

    func main() {
        // Create a new project
        projectPath := "project.pbxproj"
        project := pbxproj.NewPbxProject(projectPath)
        // Parse the project
        err := project.Parse()
        if err != nil {
            log.Fatal(err)
        }

        // Dump the project structure as JSON
        // project.Dump(os.Stdout)

        // Add some files
        err = project.AddHeaderFile("foo.h")
        if err != nil {
            log.Println(err)
        }
        err = project.AddSourceFile("foo.m")
        if err != nil {
            log.Println(err)
        }
        err = project.AddFramework("FooKit.framework")
        if err != nil {
            log.Println(err)
        }

        // Dump the modify project structure as JSON
        // project.Dump(os.Stdout)

        // Write the project back out
        // pbxproj.NewPbxWriter(&project).Write("project.pbxproj")
        _ = pbxproj.NewPbxWriter(&project).Write("modifiedProject.pbxproj")
    }
```
# Working on the parser
The .pbxProj parser(pegparser/pbxproj.go) is generated from the grammar in pegparser/pbxproj.peg by [pigeon](https://github.com/mna/pigeon).

If there's a problem parsing, you need to edit the grammar in pbxproj.peg and regenerate the parser.
```shell
# install the pigeon tool
$ go get -u github.com/mna/pigeon

$ pigeon -o pegparser/pbxproj.go pegparser/pbxproj.peg
```

# License
Apache V2
