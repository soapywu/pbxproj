/**
Licensed to the Apache Software Foundation (ASF) under one
or more contributor license agreements.  See the NOTICE file
distributed with this work for additional information
regarding copyright ownership.  The ASF licenses this file
to you under the Apache License, Version 2.0 (the
'License'); you may not use this file except in compliance
with the License.  You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing,
software distributed under the License is distributed on an
'AS IS' BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
KIND, either express or implied.  See the License for the
specific language governing permissions and limitations
under the License.
*/

package pbxproj

import (
	"path/filepath"
	"regexp"

	"github.com/soapy/pbxproj/pegparser"
)

const (
	DEFAULT_SOURCETREE         = "\"<group>\""
	DEFAULT_PRODUCT_SOURCETREE = "BUILT_PRODUCTS_DIR"
	DEFAULT_FILEENCODING       = 4
	DEFAULT_GROUP              = "Resources"
	DEFAULT_FILETYPE           = "unknown"
)

var FILETYPE_BY_EXTENSION = map[string]string{
	"a":           "archive.ar",
	"app":         "wrapper.application",
	"appex":       "wrapper.app-extension",
	"bundle":      "wrapper.plug-in",
	"dylib":       "compiled.mach-o.dylib",
	"framework":   "wrapper.framework",
	"h":           "sourcecode.c.h",
	"m":           "sourcecode.c.objc",
	"markdown":    "text",
	"mdimporter":  "wrapper.cfbundle",
	"octest":      "wrapper.cfbundle",
	"pch":         "sourcecode.c.h",
	"plist":       "text.plist.xml",
	"sh":          "text.script.sh",
	"swift":       "sourcecode.swift",
	"tbd":         "sourcecode.text-based-dylib-definition",
	"xcassets":    "folder.assetcatalog",
	"xcconfig":    "text.xcconfig",
	"xcdatamodel": "wrapper.xcdatamodel",
	"xcodeproj":   "wrapper.pb-project",
	"xctest":      "wrapper.cfbundle",
	"xib":         "file.xib",
	"strings":     "text.plist.strings",
}

func revertMap(m map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range FILETYPE_BY_EXTENSION {
		result[v] = k
	}
	return result
}

var EXTENSION_BY_FILETYPE = revertMap(FILETYPE_BY_EXTENSION)

var GROUP_BY_FILETYPE = map[string]string{
	"archive.ar":                             "Frameworks",
	"compiled.mach-o.dylib":                  "Frameworks",
	"sourcecode.text-based-dylib-definition": "Frameworks",
	"wrapper.framework":                      "Frameworks",
	"embedded.framework":                     "Embed Frameworks",
	"sourcecode.c.h":                         "Resources",
	"sourcecode.c.objc":                      "Sources",
	"sourcecode.swift":                       "Sources",
}

var PATH_BY_FILETYPE = map[string]string{
	"compiled.mach-o.dylib":                  "usr/lib/",
	"sourcecode.text-based-dylib-definition": "usr/lib/",
	"wrapper.framework":                      "System/Library/Frameworks/",
}

var SOURCETREE_BY_FILETYPE = map[string]string{
	"compiled.mach-o.dylib":                  "SDKROOT",
	"sourcecode.text-based-dylib-definition": "SDKROOT",
	"wrapper.framework":                      "SDKROOT",
}

const DEFAULT_ENCODING_VALUE = 4

var ENCODING_BY_FILETYPE = map[string]int{
	"sourcecode.c.h":     DEFAULT_ENCODING_VALUE,
	"sourcecode.c.objc":  DEFAULT_ENCODING_VALUE,
	"sourcecode.swift":   DEFAULT_ENCODING_VALUE,
	"text":               DEFAULT_ENCODING_VALUE,
	"text.plist.xml":     DEFAULT_ENCODING_VALUE,
	"text.script.sh":     DEFAULT_ENCODING_VALUE,
	"text.xcconfig":      DEFAULT_ENCODING_VALUE,
	"text.plist.strings": DEFAULT_ENCODING_VALUE,
}

var unquotedRegex = regexp.MustCompile(`(^")|("$)`)

func unquoted(text string) string {
	if text == "" {
		return text
	}
	return unquotedRegex.ReplaceAllString(text, "")
}

type PbxFileOptions struct {
	LastKnownFileType string
	CustomFramework   bool
	DefaultEncoding   int
	ExplicitFileType  string
	SourceTree        string
	Weak              bool
	CompilerFlags     string
	Embed             bool
	Sign              bool
	Target            string
	Group             string
	Plugin            bool
	VariantGroup      bool
	Link              bool
}

func newPbxFileOptions() PbxFileOptions {
	return PbxFileOptions{
		Link: true,
	}
}

type Setting struct {
	ATTRIBUTES     []string
	COMPILER_FLAGS string
}

type PbxFile struct {
	Basename          string
	FileRef           string
	LastKnownFileType string
	Group             string
	CustomFramework   bool
	Dirname           string
	Path              string
	FileEncoding      int
	ExplicitFileType  string
	SourceTree        string
	DefaultEncoding   int
	IncludeInIndex    int
	Settings          pegparser.Object
	Uuid              string
	Target            string
	Models            []*PbxFile
	CurrentModel      *PbxFile
	Plugin            bool
}

func newPbxFile(filePath string, options PbxFileOptions) *PbxFile {
	pbxfile := PbxFile{
		IncludeInIndex: 0,
	}
	pbxfile.Basename = filepath.Base(filePath)
	if options.LastKnownFileType != "" {
		pbxfile.LastKnownFileType = options.LastKnownFileType
	} else {
		pbxfile.LastKnownFileType = pbxfile.detectType(filePath)
	}
	// for custom frameworks
	if options.CustomFramework {
		pbxfile.CustomFramework = true
		pbxfile.Dirname = filepath.ToSlash(filepath.Dir(filePath))
	}

	if options.DefaultEncoding != 0 {
		pbxfile.DefaultEncoding = options.DefaultEncoding
	} else {
		pbxfile.DefaultEncoding = pbxfile.initDefaultEncoding()
	}
	pbxfile.FileEncoding = pbxfile.DefaultEncoding

	// When referencing products / build output files
	if options.ExplicitFileType != "" {
		pbxfile.ExplicitFileType = options.ExplicitFileType
		pbxfile.Basename = pbxfile.Basename + "." + pbxfile.defaultExtension()
		pbxfile.Path = ""
		pbxfile.LastKnownFileType = ""
		pbxfile.Group = ""
		pbxfile.DefaultEncoding = DEFAULT_ENCODING_VALUE
	} else {
		pbxfile.Group = pbxfile.detectGroup(options)
		pbxfile.Path = filepath.ToSlash(pbxfile.defaultPath(filePath))
	}

	if options.SourceTree != "" {
		pbxfile.SourceTree = options.SourceTree
	} else {
		pbxfile.SourceTree = pbxfile.detectSourcetree()
	}

	if options.Weak {
		if pbxfile.Settings.IsEmpty() {
			pbxfile.Settings = pegparser.NewObject()
		}
		addToObjectList(pbxfile.Settings, "ATTRIBUTES", "Weak")
	}

	if options.CompilerFlags != "" {
		if pbxfile.Settings.IsEmpty() {
			pbxfile.Settings = pegparser.NewObject()
		}
		pbxfile.Settings.Set("COMPILER_FLAGS", "\""+options.CompilerFlags+"\"")
	}

	if options.Embed && options.Sign {
		if pbxfile.Settings.IsEmpty() {
			pbxfile.Settings = pegparser.NewObject()
		}
		addToObjectList(pbxfile.Settings, "ATTRIBUTES", "CodeSignOnCopy")
	}
	return &pbxfile
}

func (pbxfile *PbxFile) defaultExtension() string {
	filetype := pbxfile.ExplicitFileType
	if pbxfile.LastKnownFileType != "" && pbxfile.LastKnownFileType != DEFAULT_FILETYPE {
		filetype = pbxfile.LastKnownFileType
	}

	extension, found := EXTENSION_BY_FILETYPE[unquoted(filetype)]
	if !found {
		panic("Unknown filetype: " + filetype)
	}

	return extension
}

func (pbxfile *PbxFile) detectType(filePath string) string {
	extension := filepath.Ext(filePath)[1:]
	filetype, found := FILETYPE_BY_EXTENSION[unquoted(extension)]

	if !found {
		return DEFAULT_FILETYPE
	}

	return filetype
}

func (pbxfile *PbxFile) detectGroup(options PbxFileOptions) string {
	extension := filepath.Ext(pbxfile.Basename)[1:]
	if extension == "xcdatamodeld" {
		return "Sources"
	}

	filetype := pbxfile.ExplicitFileType
	if pbxfile.LastKnownFileType != "" {
		filetype = pbxfile.LastKnownFileType
	}

	if options.CustomFramework && options.Embed {
		return GROUP_BY_FILETYPE["embedded.framework"]
	}

	groupName, ok := GROUP_BY_FILETYPE[unquoted(filetype)]
	if !ok {
		groupName = DEFAULT_GROUP
	}
	return groupName
}

func (pbxfile *PbxFile) initDefaultEncoding() int {
	filetype := pbxfile.ExplicitFileType
	if pbxfile.LastKnownFileType != "" {
		filetype = pbxfile.LastKnownFileType
	}
	encoding, ok := ENCODING_BY_FILETYPE[unquoted(filetype)]
	if ok {
		return encoding
	}

	return DEFAULT_ENCODING_VALUE
}

func (pbxfile *PbxFile) detectSourcetree() string {
	if pbxfile.ExplicitFileType != "" {
		return DEFAULT_PRODUCT_SOURCETREE
	}

	if pbxfile.CustomFramework {
		return DEFAULT_SOURCETREE
	}

	filetype := pbxfile.ExplicitFileType
	if pbxfile.LastKnownFileType != "" {
		filetype = pbxfile.LastKnownFileType
	}

	sourcetree, ok := SOURCETREE_BY_FILETYPE[unquoted(filetype)]
	if !ok {
		sourcetree = DEFAULT_SOURCETREE
	}
	return sourcetree
}

func (pbxfile *PbxFile) defaultPath(filePath string) string {
	if pbxfile.CustomFramework {
		return filePath
	}

	filetype := pbxfile.ExplicitFileType
	if pbxfile.LastKnownFileType != "" {
		filetype = pbxfile.LastKnownFileType
	}

	defaultPath, ok := PATH_BY_FILETYPE[unquoted(filetype)]
	if !ok {
		return filePath
	}
	return filepath.Join(defaultPath, filepath.Base(filePath))
}

func (pbxfile *PbxFile) defaultGroup() string {
	groupName, ok := GROUP_BY_FILETYPE[pbxfile.LastKnownFileType]
	if !ok {
		return DEFAULT_GROUP
	}
	return groupName
}
