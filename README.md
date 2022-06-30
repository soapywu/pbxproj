# XcodeProj

Go implement of Parser utility for xcodeproj project files, Allows you to edit xcodeproject files and write them back out.

It's heavily mimicked and inspired by [cordova-node-xcode](https://github.com/apache/cordova-node-xcode).

*Warning*: this project was not fully tested, use it at yout own risk.

# Usage
```go
    import "github.com/soapywu/pbxproj/pbxproj"

    // Create a new project
    projectPath := "project.pbxproj"
    project := pbxproj.NewPbxProject(projectPath)
    // Parse the project
    err := project.Parse()
    if err != nil {
        panic(err)
    }
    // Dump the project structure as JSON
    // project.Dump(os.Stdout)

    // Add a new Source file
    err = project.AddSourceFile("foo.m")
    if err != nil {
        panic(err)
    }

    // Write the project back out
    pbxproj.NewPbxWriter(&project).Write("project.pbxproj")
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
