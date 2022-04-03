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

type Options struct {
	LastKnownFileType string
	CustomFramework   bool
	DefaultEncoding   int
	ExplicitFileType  string
	SourceTree        string
	Weak              bool
	CompilerFlags     string
	Embed             bool
	Sign              bool
}

type Setting struct {
	ATTRIBUTES     []string
	COMPILER_FLAGS string
}

type PbxFile struct {
	basename          string
	lastKnownFileType string
	group             string
	customFramework   bool
	dirname           string
	path              string
	fileEncoding      int
	explicitFileType  string
	sourceTree        string
	defaultEncoding   int
	includeInIndex    int
	settings          Setting
}

func NewPbxFile(filePath string, options Options) *PbxFile {
	pbxfile := PbxFile{}
	pbxfile.basename = filepath.Base(filePath)
	// for custom frameworks
	if options.CustomFramework {
		pbxfile.customFramework = true
		pbxfile.dirname = filepath.ToSlash(filepath.Dir(filePath))
	}

	if options.DefaultEncoding != 0 {
		pbxfile.defaultEncoding = options.DefaultEncoding
	} else {
		pbxfile.defaultEncoding = pbxfile.getDefaultEncoding()
	}
	pbxfile.fileEncoding = pbxfile.defaultEncoding

	// When referencing products / build output files
	if options.ExplicitFileType != "" {
		pbxfile.explicitFileType = options.ExplicitFileType
		pbxfile.basename = pbxfile.basename + "." + pbxfile.defaultExtension()
		pbxfile.path = ""
		pbxfile.lastKnownFileType = ""
		pbxfile.group = ""
		pbxfile.defaultEncoding = DEFAULT_ENCODING_VALUE
	} else {
		if options.LastKnownFileType != "" {
			pbxfile.lastKnownFileType = options.LastKnownFileType
		} else {
			pbxfile.lastKnownFileType = pbxfile.detectType(filePath)
		}
		pbxfile.group = pbxfile.detectGroup(options)
		pbxfile.path = filepath.ToSlash(pbxfile.defaultPath(filePath))
	}

	if options.SourceTree != "" {
		pbxfile.sourceTree = options.SourceTree
	} else {
		pbxfile.sourceTree = pbxfile.detectSourcetree()
	}

	if options.Weak {
		pbxfile.settings = Setting{
			ATTRIBUTES: []string{"Weak"},
		}
	}

	if options.CompilerFlags != "" {
		pbxfile.settings.COMPILER_FLAGS = "\"" + options.CompilerFlags + "\""
	}

	if options.Embed && options.Sign {
		pbxfile.settings.ATTRIBUTES = append(pbxfile.settings.ATTRIBUTES, "CodeSignOnCopy")

	}
	return &pbxfile
}

func (pbxfile *PbxFile) defaultExtension() string {
	filetype := pbxfile.explicitFileType
	if pbxfile.lastKnownFileType != "" && pbxfile.lastKnownFileType != DEFAULT_FILETYPE {
		filetype = pbxfile.lastKnownFileType
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

func (pbxfile *PbxFile) detectGroup(options Options) string {
	extension := filepath.Ext(pbxfile.basename)[1:]
	if extension == "xcdatamodeld" {
		return "Sources"
	}

	filetype := pbxfile.explicitFileType
	if pbxfile.lastKnownFileType != "" {
		filetype = pbxfile.lastKnownFileType
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

func (pbxfile *PbxFile) getDefaultEncoding() int {
	filetype := pbxfile.explicitFileType
	if pbxfile.lastKnownFileType != "" {
		filetype = pbxfile.lastKnownFileType
	}
	encoding, ok := ENCODING_BY_FILETYPE[unquoted(filetype)]
	if !ok {
		panic("Unknown filetype: " + filetype)
	}

	return encoding
}

func (pbxfile *PbxFile) detectSourcetree() string {
	if pbxfile.explicitFileType != "" {
		return DEFAULT_PRODUCT_SOURCETREE
	}

	if pbxfile.customFramework {
		return DEFAULT_SOURCETREE
	}

	filetype := pbxfile.explicitFileType
	if pbxfile.lastKnownFileType != "" {
		filetype = pbxfile.lastKnownFileType
	}

	sourcetree, ok := SOURCETREE_BY_FILETYPE[unquoted(filetype)]
	if !ok {
		sourcetree = DEFAULT_SOURCETREE
	}
	return sourcetree
}

func (pbxfile *PbxFile) defaultPath(filePath string) string {
	if pbxfile.customFramework {
		return filePath
	}

	filetype := pbxfile.explicitFileType
	if pbxfile.lastKnownFileType != "" {
		filetype = pbxfile.lastKnownFileType
	}

	defaultPath, ok := PATH_BY_FILETYPE[unquoted(filetype)]
	if !ok {
		return filePath
	}
	return filepath.Join(defaultPath, filepath.Base(filePath))
}

func (pbxfile *PbxFile) defaultGroup() string {
	groupName, ok := GROUP_BY_FILETYPE[pbxfile.lastKnownFileType]
	if !ok {
		return DEFAULT_GROUP
	}
	return groupName
}
