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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gofrs/uuid"
	"github.com/soapy/pbxproj/pegparser"
)

type CommentValue struct {
	Value   string
	Comment string
}

func (c CommentValue) ToObject() pegparser.Object {
	return pegparser.NewObjectWithData([]pegparser.SliceItem{
		pegparser.NewObjectItem("value", c.Value),
		pegparser.NewObjectItem("comment", c.Comment),
	})
}

type PbxProjectWriterOption struct {
}

type PbxProject struct {
	filePath                       string
	pbxContents                    pegparser.Object
	topProjectSection              pegparser.Object
	pbxObjectSection               pegparser.Object
	pbxGroupSection                pegparser.Object
	pbxProjectSection              pegparser.Object
	pbxBuildFileSection            pegparser.Object
	pbxXCBuildConfigurationSection pegparser.Object
	pbxFileReferenceSection        pegparser.Object
	pbxNativeTargetSection         pegparser.Object
	xcVersionGroupSection          pegparser.Object
	pbxXCConfigurationListSection  pegparser.Object
	pbxTargetDependencySection     pegparser.Object
	pbxContainerItemProxySection   pegparser.Object
	uuids                          map[string]struct{}
	pbxFileReferences              map[string]struct{}
	pbxFileNoCommentReferences     map[string]*PbxFile
}

func NewPbxProject(filename string) PbxProject {
	return PbxProject{
		filePath:                   filename,
		uuids:                      make(map[string]struct{}),
		pbxFileReferences:          make(map[string]struct{}),
		pbxFileNoCommentReferences: make(map[string]*PbxFile),
	}
}

func (p *PbxProject) Contents() pegparser.Object {
	return p.pbxContents
}

func (p *PbxProject) Parse() error {
	data, err := ioutil.ReadFile(p.filePath)
	if err != nil {
		return err
	}

	contents, err := pegparser.ParseReader("", bytes.NewReader(data))
	if err != nil {
		return err
	}
	p.pbxContents = contents.(pegparser.Object)
	p.initSections()
	p.buildExistUuids()
	return nil
}

func (p *PbxProject) Dump(writer io.Writer) error {
	buffer := bytes.NewBuffer([]byte{})
	jsonEncoder := json.NewEncoder(buffer)
	jsonEncoder.SetEscapeHTML(false)
	jsonEncoder.SetIndent("", "  ")
	_ = jsonEncoder.Encode(p.Contents())
	_, _ = writer.Write(buffer.Bytes())
	return nil
}

func (p *PbxProject) initSections() {
	p.topProjectSection = p.pbxContents.GetObject("project")
	p.pbxObjectSection = p.topProjectSection.GetObject("objects")
	p.pbxGroupSection = p.topProjectSection.GetObject("PBXGroup")
	p.pbxProjectSection = p.pbxObjectSection.GetObject("PBXProject")
	p.pbxBuildFileSection = p.pbxObjectSection.GetObject("PBXBuildFile")
	p.pbxXCBuildConfigurationSection = p.pbxObjectSection.GetObject("XCBuildConfiguration")
	p.pbxFileReferenceSection = p.pbxObjectSection.GetObject("PBXFileReference")
	p.pbxNativeTargetSection = p.pbxObjectSection.GetObject("PBXNativeTarget")
	p.pbxTargetDependencySection = p.pbxObjectSection.GetObject("PBXTargetDependency")     // @fixme if not exist create when add
	p.pbxContainerItemProxySection = p.pbxObjectSection.GetObject("PBXContainerItemProxy") // @fixme if not exist create when add
	xcVersionGroupSection := p.pbxObjectSection.GetObject("XCVersionGroup")
	if xcVersionGroupSection.IsEmpty() {
		xcVersionGroupSection = pegparser.NewObject()
		p.pbxObjectSection.Set("XCVersionGroup", xcVersionGroupSection)
	}
	p.xcVersionGroupSection = xcVersionGroupSection

	pbxXCConfigurationListSection := p.pbxObjectSection.GetObject("XCConfigurationList")
	if pbxXCConfigurationListSection.IsEmpty() {
		pbxXCConfigurationListSection = pegparser.NewObject()
		p.pbxObjectSection.Set("XCConfigurationList", pbxXCConfigurationListSection)
	}
	p.pbxXCConfigurationListSection = pbxXCConfigurationListSection
}

func (p *PbxProject) buildExistUuids() {
	uuids := make(map[string]struct{})
	p.pbxObjectSection.Foreach(func(_ string, v interface{}) pegparser.IterateActionType {
		fileSection := v.(pegparser.Object)
		fileSection.ForeachWithFilter(func(key string, value interface{}) pegparser.IterateActionType {
			if len(key) == 24 { // @fixme more exact check
				uuids[key] = struct{}{}
			}
			return pegparser.IterateActionContinue
		}, nonCommentsFilter)
		return pegparser.IterateActionContinue
	})

	p.uuids = uuids
}

func (p *PbxProject) WriteSync(options PbxProjectWriterOption) error {
	// writer := NewPbxWriter(p.contents, options)
	// return writer.WriteSync(p.filePath, p.contents)
	return nil
}

func (p *PbxProject) generateUuid() string {
	u, _ := uuid.NewV4()
	newUUID := strings.ToUpper(strings.ReplaceAll(u.String(), "-", "")[0:24])

	_, found := p.uuids[newUUID]
	if found {
		return p.generateUuid()
	} else {
		p.uuids[newUUID] = struct{}{}
		return newUUID
	}
}

func parseFileVariadicParams(params ...interface{}) (options PbxFileOptions, group string) {
	for _, param := range params {
		switch param := param.(type) {
		case PbxFileOptions:
			options = param
		case string:
			group = param
		}
	}
	return
}

func (p *PbxProject) addPluginFile(filePath string, options PbxFileOptions) (*PbxFile, error) {
	pbxfile := newPbxFile(filePath, options)
	pbxfile.Plugin = true
	p.correctForPluginsPath(pbxfile)
	if p.hasFile(pbxfile.Path) {
		return nil, fmt.Errorf("file already exist: %s", pbxfile.Path)
	}
	pbxfile.FileRef = p.generateUuid()
	p.addToPbxFileReferenceSection(pbxfile) // PBXFileReference
	p.addToPluginsPbxGroup(pbxfile)         // PBXGroup
	return pbxfile, nil
}

func (p *PbxProject) AddPluginFile(filePath string, params ...interface{}) error {
	options, _ := parseFileVariadicParams(params...)
	_, err := p.addPluginFile(filePath, options)
	return err
}

func (p *PbxProject) removePluginFile(filePath string, options PbxFileOptions) *PbxFile {
	pbxfile := newPbxFile(filePath, options)
	pbxfile.Plugin = true
	p.correctForPluginsPath(pbxfile)
	p.removeFromPbxFileReferenceSection(pbxfile) // PBXFileReference
	p.removeFromPluginsPbxGroup(pbxfile)         // PBXGroup
	return pbxfile
}

func (p *PbxProject) RemovePluginFile(filePath string, params ...interface{}) error {
	options, _ := parseFileVariadicParams(params...)
	_ = p.removePluginFile(filePath, options)
	return nil
}

func (p *PbxProject) addProductFile(filePath string, options PbxFileOptions) *PbxFile {
	pbxfile := newPbxFile(filePath, options)
	pbxfile.IncludeInIndex = 0
	pbxfile.FileRef = p.generateUuid()
	pbxfile.Target = options.Target
	pbxfile.Group = options.Group
	pbxfile.Uuid = p.generateUuid()
	pbxfile.Path = pbxfile.Basename
	p.addToPbxFileReferenceSection(pbxfile)
	p.addToProductsPbxGroup(pbxfile) // PBXGroup
	return pbxfile
}
func (p *PbxProject) AddProductFile(filePath string, params ...interface{}) error {
	options, _ := parseFileVariadicParams(params...)
	_ = p.addProductFile(filePath, options)
	return nil
}
func (p *PbxProject) RemoveProductFile(filePath string, params ...interface{}) error {
	options, _ := parseFileVariadicParams(params...)
	pbxfile := newPbxFile(filePath, options)
	p.removeFromPbxFileReferenceSection(pbxfile)
	p.removeFromProductsPbxGroup(pbxfile) // PBXGroup
	return nil
}

func (p *PbxProject) AddSourceFile(filePath string, params ...interface{}) error {
	options, group := parseFileVariadicParams(params...)
	var pbxfile *PbxFile
	var err error
	if group != "" {
		pbxfile, err = p.addFile(filePath, group, options)
	} else {
		pbxfile, err = p.addPluginFile(filePath, options)
	}

	if err != nil {
		return err
	}

	pbxfile.Target = options.Target
	pbxfile.Uuid = p.generateUuid()
	p.addToPbxBuildFileSection(pbxfile)  // PBXBuildFile
	p.addToPbxSourcesBuildPhase(pbxfile) // PBXSourcesBuildPhase
	return nil
}
func (p *PbxProject) RemoveSourceFile(filePath string, params ...interface{}) error {
	options, group := parseFileVariadicParams(params...)
	var pbxfile *PbxFile
	if group != "" {
		pbxfile = p.removeFile(filePath, group, options)
	} else {
		pbxfile = p.removePluginFile(filePath, options)
	}

	pbxfile.Target = options.Target
	p.removeFromPbxBuildFileSection(pbxfile)  // PBXBuildFile
	p.removeFromPbxSourcesBuildPhase(pbxfile) // PBXSourcesBuildPhase
	return nil
}

func (p *PbxProject) AddHeaderFile(filePath string, params ...interface{}) error {
	options, group := parseFileVariadicParams(params...)
	if group != "" {
		return p.AddFile(filePath, group, options)
	} else {
		return p.AddPluginFile(filePath, options)
	}
}

func (p *PbxProject) AddHeaderFileWithOptions(filePath string, params ...interface{}) error {
	options, group := parseFileVariadicParams(params...)
	if group != "" {
		return p.AddFile(filePath, group, options)
	} else {
		return p.AddPluginFile(filePath, options)
	}
}
func (p *PbxProject) RemoveHeaderFile(filePath string, params ...interface{}) error {
	options, group := parseFileVariadicParams(params...)
	if group != "" {
		return p.RemoveFile(filePath, group, options)
	} else {
		return p.RemovePluginFile(filePath, options)
	}
}
func (p *PbxProject) AddResourceFile(filePath string, params ...interface{}) error {
	options, group := parseFileVariadicParams(params...)
	var pbxfile *PbxFile
	var err error

	if options.Plugin {
		pbxfile, err = p.addPluginFile(filePath, options)
		if err != nil {
			return err
		}
	} else {
		pbxfile = newPbxFile(filePath, options)
		if p.hasFile(pbxfile.Path) {
			return fmt.Errorf("file %s already exists", filePath)
		}
	}

	pbxfile.Uuid = p.generateUuid()
	pbxfile.Target = options.Target
	if !options.Plugin {
		p.correctForResourcesPath(pbxfile)
		pbxfile.FileRef = p.generateUuid()
	}

	if !options.VariantGroup {
		p.addToPbxBuildFileSection(pbxfile)    // PBXBuildFile
		p.addToPbxResourcesBuildPhase(pbxfile) // PBXResourcesBuildPhase
	}

	if !options.Plugin {
		p.addToPbxFileReferenceSection(pbxfile) // PBXFileReference
		if group != "" {
			if !p.getPBXGroupByKey(group).IsEmpty() {
				p.addToPbxGroup(pbxfile, group) //Group other than Resources (i.e. "splash")
			} else if !p.getPBXVariantGroupByKey(group).IsEmpty() {
				p.addToPbxVariantGroup(pbxfile, group) // PBXVariantGroup
			}
		} else {
			p.addToResourcesPbxGroup(pbxfile) // PBXGroup
		}
	}
	return nil
}
func (p *PbxProject) RemoveResourceFile(filePath string, params ...interface{}) error {
	options, group := parseFileVariadicParams(params...)
	pbxfile := newPbxFile(filePath, options)
	pbxfile.Target = options.Target

	if options.Plugin {
		p.correctForResourcesPath(pbxfile)
		p.removeFromPluginsPbxGroup(pbxfile)
	}
	p.correctForResourcesPath(pbxfile)
	p.removeFromPbxBuildFileSection(pbxfile)     // PBXBuildFile
	p.removeFromPbxFileReferenceSection(pbxfile) // PBXFileReference
	if group != "" {
		if !p.getPBXGroupByKey(group).IsEmpty() {
			p.removeFromPbxGroup(pbxfile, group) //Group other than Resources (i.e. "splash")
		} else if !p.getPBXVariantGroupByKey(group).IsEmpty() {
			p.removeFromPbxVariantGroup(pbxfile, group) // PBXVariantGroup
		}
	} else {
		p.removeFromResourcesPbxGroup(pbxfile) // PBXGroup
	}
	p.removeFromPbxResourcesBuildPhase(pbxfile) // PBXResourcesBuildPhase
	return nil
}

func (p *PbxProject) AddFramework(filePath string, params ...interface{}) error {
	options, _ := parseFileVariadicParams(params...)
	customFramework := options.CustomFramework
	link := options.Link
	embed := options.Embed

	options.Embed = false
	pbxfile := newPbxFile(filePath, options)
	pbxfile.Uuid = p.generateUuid()
	pbxfile.FileRef = p.generateUuid()
	pbxfile.Target = options.Target

	if p.hasFile(pbxfile.Path) {
		return fmt.Errorf("Framework %s already exists", pbxfile.Path)
	}
	p.addToPbxBuildFileSection(pbxfile)     // PBXBuildFile
	p.addToPbxFileReferenceSection(pbxfile) // PBXFileReference
	p.addToFrameworksPbxGroup(pbxfile)      // PBXGroup

	if link {
		p.addToPbxFrameworksBuildPhase(pbxfile) // PBXFrameworksBuildPhase
	}

	if customFramework {
		p.addToFrameworkSearchPaths(pbxfile)
		if embed {
			options.Embed = true
			embeddedPbxFile := newPbxFile(filePath, options)
			embeddedPbxFile.Uuid = p.generateUuid()
			embeddedPbxFile.FileRef = pbxfile.FileRef

			//keeping a separate PBXBuildFile entry for Embed Frameworks
			p.addToPbxBuildFileSection(embeddedPbxFile)          // PBXBuildFile
			p.addToPbxEmbedFrameworksBuildPhase(embeddedPbxFile) // PBXCopyFilesBuildPhase
		}
	}
	return nil
}
func (p *PbxProject) RemoveFramework(filePath string, params ...interface{}) error {
	options, _ := parseFileVariadicParams(params...)
	options.Embed = false
	pbxfile := newPbxFile(filePath, options)
	pbxfile.Target = options.Target

	p.removeFromPbxBuildFileSection(pbxfile)     // PBXBuildFile
	p.removeFromPbxFileReferenceSection(pbxfile) // PBXFileReference
	p.removeFromFrameworksPbxGroup(pbxfile)      // PBXGroup
	p.removeFromPbxFrameworksBuildPhase(pbxfile) // PBXFrameworksBuildPhase

	if options.CustomFramework {
		p.removeFromFrameworkSearchPaths(pbxfile)
	}

	options.Embed = true
	embeddedPbxFile := newPbxFile(filePath, options)
	embeddedPbxFile.FileRef = pbxfile.FileRef
	p.removeFromPbxBuildFileSection(embeddedPbxFile)          // PBXBuildFile
	p.removeFromPbxEmbedFrameworksBuildPhase(embeddedPbxFile) // PBXCopyFilesBuildPhase
	return nil
}

func (p *PbxProject) AddCopyfile(filePath string, params ...interface{}) error {
	options, _ := parseFileVariadicParams(params...)
	pbxfile := newPbxFile(filePath, options)
	// catch duplicates
	if p.hasFile(pbxfile.Path) {
		pbxfile = p.getFile(pbxfile.Path)
	}
	pbxfile.Uuid = p.generateUuid()
	pbxfile.FileRef = pbxfile.Uuid
	pbxfile.Target = options.Target
	p.addToPbxBuildFileSection(pbxfile)     // PBXBuildFile
	p.addToPbxFileReferenceSection(pbxfile) // PBXFileReference
	p.addToPbxCopyfilesBuildPhase(pbxfile)  // PBXCopyFilesBuildPhase
	return nil
}

func (p *PbxProject) pbxCopyfilesBuildPhaseObj(target string) pegparser.Object {
	return p.buildPhaseObject("PBXCopyFilesBuildPhase", "Copy Files", target)
}

func (p *PbxProject) addToPbxCopyfilesBuildPhase(pbxfile *PbxFile) {
	sources := p.buildPhaseObject("PBXCopyFilesBuildPhase", "Copy Files", pbxfile.Target)
	addToObjectList(sources, "files", pbxBuildPhaseObj(pbxfile))
}

func (p *PbxProject) RemoveCopyfile(filePath string, params ...interface{}) error {
	options, _ := parseFileVariadicParams(params...)
	pbxfile := newPbxFile(filePath, options)
	pbxfile.Target = options.Target
	p.removeFromPbxBuildFileSection(pbxfile)     // PBXBuildFile
	p.removeFromPbxFileReferenceSection(pbxfile) // PBXFileReference
	p.removeFromPbxCopyfilesBuildPhase(pbxfile)  // PBXFrameworksBuildPhase
	return nil
}

func (p *PbxProject) removeFromPbxCopyfilesBuildPhase(pbxfile *PbxFile) {
	sources := p.pbxCopyfilesBuildPhaseObj(pbxfile.Target)
	pbxfileComment := longComment(pbxfile)
	removeFromObjectList(sources, "files", func(v interface{}) bool {
		comment := v.(pegparser.Object).GetString("comment")
		return comment == pbxfileComment
	}, false)

}

func (p *PbxProject) AddStaticLibrary(filePath string, params ...interface{}) error {
	options, _ := parseFileVariadicParams(params...)
	var pbxfile *PbxFile
	var err error
	if options.Plugin {
		pbxfile, err = p.addPluginFile(filePath, options)
		if err != nil {
			return err
		}
	} else {
		pbxfile = newPbxFile(filePath, options)
		if p.hasFile(pbxfile.Path) {
			return fmt.Errorf("File %s already exists", filePath)
		}
	}

	pbxfile.Uuid = p.generateUuid()
	pbxfile.Target = options.Target
	if !options.Plugin {
		pbxfile.FileRef = p.generateUuid()
		p.addToPbxFileReferenceSection(pbxfile) // PBXFileReference
	}
	p.addToPbxBuildFileSection(pbxfile)     // PBXBuildFile
	p.addToPbxFrameworksBuildPhase(pbxfile) // PBXFrameworksBuildPhase
	p.addToLibrarySearchPaths(pbxfile)      // make sure it gets built!
	return nil
}

// helper addition functions
func (p *PbxProject) addToPbxBuildFileSection(pbxfile *PbxFile) {
	p.pbxBuildFileSection.Set(pbxfile.Uuid, pbxBuildFileObj(pbxfile))
	p.pbxBuildFileSection.Set(toCommentKey(pbxfile.Uuid), pbxBuildFileComment(pbxfile))
}

func (p *PbxProject) removeFromPbxBuildFileSection(pbxfile *PbxFile) {
	p.pbxGroupSection.ForeachWithFilter(func(key string, value interface{}) pegparser.IterateActionType {
		if value.(pegparser.Object).GetString(toCommentKey("FileRef")) == pbxfile.Basename {
			p.pbxBuildFileSection.Delete(key)
			p.pbxBuildFileSection.Delete(toCommentKey(key))
		}
		return pegparser.IterateActionContinue
	}, nonCommentsFilter)
}

type FileReferenceAndBase struct {
	FileRef  string
	Basename string
}

func (p *PbxProject) AddPbxGroup(filePathsArray []string, name, path, sourceTree string) {
	pbxGroupUuid := p.generateUuid()
	pbxGroup := pegparser.NewObjectWithData([]pegparser.SliceItem{
		pegparser.NewObjectItem("isa", "PBXGroup"),
		pegparser.NewObjectItem("children", []interface{}{}),
		pegparser.NewObjectItem("name", name),
		pegparser.NewObjectItem("sourceTree", sourceTree),
	})
	if path != "" {
		pbxGroup.Set("path", path)
	}
	if sourceTree == "" {
		pbxGroup.Set("sourceTree", `"<group>"`)
	}

	filePathToReference := map[string]CommentValue{}

	p.pbxFileReferenceSection.ForeachWithFilter(func(key string, value interface{}) pegparser.IterateActionType {
		basename := value.(string)
		if basename == "" {
			return pegparser.IterateActionContinue
		}

		fileReferenceKey := fromCommentKey(key)
		path := p.pbxFileReferenceSection.GetObject(fileReferenceKey).GetString("path")
		if path == "" {
			return pegparser.IterateActionContinue
		}

		filePathToReference[path] = CommentValue{
			Value:   fileReferenceKey,
			Comment: basename,
		}
		return pegparser.IterateActionContinue
	}, onlyCommentsFilter)

	for _, filePath := range filePathsArray {
		filePathQuoted := `"` + filePath + `"`
		commentValue, found := filePathToReference[filePath]
		if found {
			addToObjectList(pbxGroup, "children", commentValue.ToObject())
			continue
		} else {
			commentValue, found := filePathToReference[filePathQuoted]
			if found {
				addToObjectList(pbxGroup, "children", commentValue.ToObject())
				continue
			}
		}

		pbxfile := newPbxFile(filePath, newPbxFileOptions())
		pbxfile.Uuid = p.generateUuid()
		pbxfile.FileRef = p.generateUuid()
		p.addToPbxFileReferenceSection(pbxfile) // PBXFileReference
		p.addToPbxBuildFileSection(pbxfile)     // PBXBuildFile
		addToObjectList(pbxGroup, "children", CommentValue{
			Value:   pbxfile.FileRef,
			Comment: pbxfile.Basename,
		}.ToObject())
	}

	if !p.pbxGroupSection.IsEmpty() {
		p.pbxGroupSection.Set(pbxGroupUuid, pbxGroup)
		p.pbxGroupSection.Set(toCommentKey(pbxGroupUuid), name)
	}
}

func (p *PbxProject) RemovePbxGroup(groupName string) {
	p.pbxGroupSection.ForeachWithFilter(func(key string, value interface{}) pegparser.IterateActionType {
		if value.(string) == groupName {
			p.pbxGroupSection.Delete(key)
			p.pbxGroupSection.Delete(fromCommentKey(key))
			return pegparser.IterateActionBreak
		}
		return pegparser.IterateActionContinue
	}, onlyCommentsFilter)
}

func (p *PbxProject) addToPbxProjectSection(uuid string, target pegparser.Object) {
	newTarget := CommentValue{
		Value:   uuid,
		Comment: pbxNativeTargetComment(target),
	}
	project := p.getFirstProject()
	addToObjectList(project.Object, "targets", newTarget.ToObject())
}

func (p *PbxProject) addToPbxNativeTargetSection(uuid string, target pegparser.Object) {
	p.pbxNativeTargetSection.Set(uuid, target)
	p.pbxNativeTargetSection.Set(toCommentKey(uuid), target.GetString("name"))
}

func (p *PbxProject) addToPbxFileReferenceSection(pbxfile *PbxFile) {
	p.pbxFileReferenceSection.Set(pbxfile.FileRef, newPbxFileReferenceObj(pbxfile))
	p.pbxFileReferenceSection.Set(toCommentKey(pbxfile.FileRef), pbxFileReferenceComment(pbxfile))
}

func (p *PbxProject) removeFromPbxFileReferenceSection(pbxfile *PbxFile) {
	refObj := newPbxFileReferenceObj(pbxfile)
	refObjName := refObj.GetString("name")
	refObjPath := refObj.GetString("path")

	p.pbxFileReferenceSection.ForeachWithFilter(func(key string, val interface{}) pegparser.IterateActionType {
		pbxfile := val.(pegparser.Object)
		name := pbxfile.GetString("name")
		path := pbxfile.GetString("path")
		if name == refObjName || `"`+name+`"` == refObjName || path == refObjPath || `"`+path+`"` == refObjPath {
			p.pbxFileReferenceSection.Delete(key)
			p.pbxFileReferenceSection.Delete(toCommentKey(pbxfile.GetString("FileRef")))
			return pegparser.IterateActionBreak
		}

		return pegparser.IterateActionContinue
	}, nonCommentsFilter)
}

func (p *PbxProject) addToXcVersionGroupSection(pbxfile *PbxFile) error {
	if pbxfile.Models == nil || pbxfile.CurrentModel == nil {
		return fmt.Errorf("Cannot create a XCVersionGroup section from not a data model document file")
	}
	fileRefs := make([]string, len(pbxfile.Models))
	for i, model := range pbxfile.Models {
		fileRefs[i] = model.FileRef
	}

	if !p.xcVersionGroupSection.Has(pbxfile.FileRef) {
		p.xcVersionGroupSection.Set(pbxfile.FileRef, pegparser.NewObjectWithData([]pegparser.ObjectItem{
			pegparser.NewObjectItem("isa", "XCVersionGroup"),
			pegparser.NewObjectItem("children", fileRefs),
			pegparser.NewObjectItem("currentVersion", pbxfile.CurrentModel.FileRef),
			pegparser.NewObjectItem("name", filepath.Base(pbxfile.Path)),
			pegparser.NewObjectItem("path", pbxfile.Path),
			pegparser.NewObjectItem("sourceTree", `"<group>"`),
			pegparser.NewObjectItem("versionGroupType", "wrapper.xcdatamodel"),
		}))

		p.xcVersionGroupSection.Set(toCommentKey(pbxfile.FileRef), filepath.Base(pbxfile.Path))
	}
	return nil
}

func (p *PbxProject) addToPbxGroup(pbxfile *PbxFile, groupName string) {
	group := p.pbxGroupByName(groupName)
	if group.IsEmpty() {
		p.AddPbxGroup([]string{pbxfile.Path}, groupName, "", "")
	} else {
		addToObjectList(group, "children", pbxGroupChild(pbxfile))
	}
}

func (p *PbxProject) removeFromPbxGroup(pbxfile *PbxFile, groupName string) {
	group := p.pbxGroupByName(groupName)
	if group.IsEmpty() {
		return
	}

	removeFromObjectList(group, "children", func(child interface{}) bool {
		childObj := child.(pegparser.Object)
		return childObj.GetString("value") == pbxfile.FileRef && childObj.GetString("comment") == pbxfile.Basename
	}, false)
}

func (p *PbxProject) addToPluginsPbxGroup(pbxfile *PbxFile) {
	p.addToPbxGroup(pbxfile, "Plugins")
}

func (p *PbxProject) removeFromPluginsPbxGroup(pbxfile *PbxFile) {
	p.removeFromPbxGroup(pbxfile, "Plugins")
}

func (p *PbxProject) addToResourcesPbxGroup(pbxfile *PbxFile) {
	p.addToPbxGroup(pbxfile, "Resources")
}

func (p *PbxProject) removeFromResourcesPbxGroup(pbxfile *PbxFile) {
	p.removeFromPbxGroup(pbxfile, "Resources")
}

func (p *PbxProject) addToFrameworksPbxGroup(pbxfile *PbxFile) {
	p.addToPbxGroup(pbxfile, "Frameworks")
}

func (p *PbxProject) removeFromFrameworksPbxGroup(pbxfile *PbxFile) {
	p.removeFromPbxGroup(pbxfile, "Frameworks")
}

func (p *PbxProject) addToProductsPbxGroup(pbxfile *PbxFile) {
	p.addToPbxGroup(pbxfile, "Products")
}

func (p *PbxProject) removeFromProductsPbxGroup(pbxfile *PbxFile) {
	p.removeFromPbxGroup(pbxfile, "Products")
}

func (p *PbxProject) addToPbxBuildPhase(source pegparser.Object, pbxfile *PbxFile) {
	addToObjectList(source, "files", pbxBuildPhaseObj(pbxfile))
}

func (p *PbxProject) removeFromPbxBuildPhase(source pegparser.Object, pbxfile *PbxFile) {
	comment := longComment(pbxfile)
	removeFromObjectList(source, "files", func(file interface{}) bool {
		return file.(pegparser.Object).GetString("comment") == comment
	}, false)
}

func (p *PbxProject) addToPbxEmbedFrameworksBuildPhase(pbxfile *PbxFile) {
	p.addToPbxBuildPhase(p.pbxEmbedFrameworksBuildPhaseObj(pbxfile.Target), pbxfile)
}

func (p *PbxProject) removeFromPbxEmbedFrameworksBuildPhase(pbxfile *PbxFile) {
	p.removeFromPbxBuildPhase(p.pbxEmbedFrameworksBuildPhaseObj(pbxfile.Target), pbxfile)
}

func (p *PbxProject) addToPbxSourcesBuildPhase(pbxfile *PbxFile) {
	p.addToPbxBuildPhase(p.pbxSourcesBuildPhaseObj(pbxfile.Target), pbxfile)
}

func (p *PbxProject) removeFromPbxSourcesBuildPhase(pbxfile *PbxFile) {
	p.removeFromPbxBuildPhase(p.pbxSourcesBuildPhaseObj(pbxfile.Target), pbxfile)
}

func (p *PbxProject) addToPbxResourcesBuildPhase(pbxfile *PbxFile) {
	p.addToPbxBuildPhase(p.pbxResourcesBuildPhaseObj(pbxfile.Target), pbxfile)
}

func (p *PbxProject) removeFromPbxResourcesBuildPhase(pbxfile *PbxFile) {
	p.removeFromPbxBuildPhase(p.pbxResourcesBuildPhaseObj(pbxfile.Target), pbxfile)
}

func (p *PbxProject) addToPbxFrameworksBuildPhase(pbxfile *PbxFile) {
	p.addToPbxBuildPhase(p.pbxFrameworksBuildPhaseObj(pbxfile.Target), pbxfile)
}

func (p *PbxProject) removeFromPbxFrameworksBuildPhase(pbxfile *PbxFile) {
	p.removeFromPbxBuildPhase(p.pbxFrameworksBuildPhaseObj(pbxfile.Target), pbxfile)
}

func (p *PbxProject) addXCConfigurationList(configurationObjectsArray []pegparser.Object, defaultConfigurationName, comment string) pegparser.ObjectWithUUID {
	xcConfigurationListUuid := p.generateUuid()
	buildConfigurations := make([]pegparser.Object, 0)

	xcConfigurationList := pegparser.NewObjectWithData([]pegparser.SliceItem{
		pegparser.NewObjectItem("isa", "XCConfigurationList"),
		pegparser.NewObjectItem("defaultConfigurationIsVisible", 0),
		pegparser.NewObjectItem("defaultConfigurationName", defaultConfigurationName),
	})
	for _, configuration := range configurationObjectsArray {
		configurationUuid := p.generateUuid()
		p.pbxXCBuildConfigurationSection.Set(configurationUuid, configuration)
		p.pbxXCBuildConfigurationSection.Set(toCommentKey(configurationUuid), configuration.GetString("name"))
		buildConfigurations = append(buildConfigurations, CommentValue{
			Value:   configurationUuid,
			Comment: configuration.GetString("name"),
		}.ToObject())
	}
	xcConfigurationList.Set("buildConfigurations", buildConfigurations)

	p.pbxXCConfigurationListSection.Set(xcConfigurationListUuid, xcConfigurationList)
	p.pbxXCConfigurationListSection.Set(toCommentKey(xcConfigurationListUuid), comment)

	return pegparser.ObjectWithUUID{
		UUID:   xcConfigurationListUuid,
		Object: xcConfigurationList,
	}
}

func (p *PbxProject) AddTargetDependency(target string, dependencyTargets []string) {
	if target == "" {
		return
	}

	if !p.pbxNativeTargetSection.Has(target) {
		fmt.Printf("Target %s not found.\n", target)
		return
	}

	for _, dependencyTarget := range dependencyTargets {
		if !p.pbxNativeTargetSection.Has(dependencyTarget) {
			fmt.Printf("dependencyTarget %s not found.\n", dependencyTarget)
			return
		}
	}
	targetObj := p.pbxNativeTargetSection.GetObject(target)
	if targetObj.IsEmpty() {
		return
	}

	for _, dependencyTargetUuid := range dependencyTargets {
		targetDependencyUuid := p.generateUuid()
		itemProxyUuid := p.generateUuid()
		itemProxy := pegparser.NewObjectWithData([]pegparser.SliceItem{
			pegparser.NewObjectItem("isa", "PBXContainerItemProxy"),
			pegparser.NewObjectItem("containerPortal", p.topProjectSection.GetString("rootObject")),
			pegparser.NewObjectItem(toCommentKey("containerPortal"), p.topProjectSection.GetString(toCommentKey("rootObject"))),
			pegparser.NewObjectItem("proxyType", 1),
			pegparser.NewObjectItem("remoteGlobalIDString", dependencyTargetUuid),
			pegparser.NewObjectItem("remoteInfo", p.pbxNativeTargetSection.GetObject(dependencyTargetUuid).GetString("name")),
		})

		targetDependency := pegparser.NewObjectWithData([]pegparser.SliceItem{
			pegparser.NewObjectItem("isa", "PBXTargetDependency"),
			pegparser.NewObjectItem("target", dependencyTargetUuid),
			pegparser.NewObjectItem(toCommentKey("target"), p.pbxNativeTargetSection.GetString(toCommentKey(dependencyTargetUuid))),
			pegparser.NewObjectItem("targetProxy", itemProxyUuid),
			pegparser.NewObjectItem(toCommentKey("targetProxy"), "PBXContainerItemProxy"),
		})

		p.pbxContainerItemProxySection.Set(itemProxyUuid, itemProxy)
		p.pbxContainerItemProxySection.Set(toCommentKey(itemProxyUuid), "pbxContainerItemProxy")
		p.pbxTargetDependencySection.Set(targetDependencyUuid, targetDependency)
		p.pbxTargetDependencySection.Set(toCommentKey(targetDependencyUuid), "pbxTargetDependency")
		addToObjectList(targetObj, "dependencies", CommentValue{
			Value:   targetDependencyUuid,
			Comment: "pbxTargetDependency",
		}.ToObject())
	}
}

func (p *PbxProject) AddBuildPhase(filePathsArray []string, buildPhaseType, comment, target string, optionsOrFolderType interface{}, subfolderPath string) {
	buildPhaseUuid := p.generateUuid()
	buildPhaseTargetUuid := target
	if target == "" {
		buildPhaseTargetUuid = p.getFirstTarget().UUID
	}
	commentKey := toCommentKey(buildPhaseUuid)

	buildPhase := pegparser.NewObjectWithData([]pegparser.SliceItem{
		pegparser.NewObjectItem("isa", buildPhaseType),
		pegparser.NewObjectItem("buildActionMask", 2147483647),
		pegparser.NewObjectItem("files", []interface{}{}),
		pegparser.NewObjectItem("runOnlyForDeploymentPostprocessing", 0),
	})

	filePathToBuildFile := map[string]*PbxFile{}
	if buildPhaseType == "PBXCopyFilesBuildPhase" {
		folderType, ok := optionsOrFolderType.(string)
		if !ok {
			fmt.Println("optionsOrFolderType is not string")
			return
		}
		buildPhase = pbxCopyFilesBuildPhaseObj(buildPhase, folderType, subfolderPath, comment)
	} else if buildPhaseType == "PBXShellScriptBuildPhase" {
		options, ok := optionsOrFolderType.(pbxShellScriptBuildPhaseObjOptions)
		if !ok {
			fmt.Println("optionsOrFolderType is not pbxShellScriptBuildPhaseObjOptions")
			return
		}
		buildPhase = pbxShellScriptBuildPhaseObj(buildPhase, options, comment)
	}

	buildPhaseSection := p.pbxObjectSection.GetObject(buildPhaseType)
	if buildPhaseSection.IsEmpty() {
		buildPhaseSection = pegparser.NewObject()
		p.pbxObjectSection.Set(buildPhaseType, buildPhaseSection)
	}
	if !buildPhaseSection.Has(buildPhaseUuid) {
		buildPhaseSection.Set(buildPhaseUuid, buildPhase)
		buildPhaseSection.Set(commentKey, comment)
	}

	targetObj := p.pbxNativeTargetSection.GetObject(buildPhaseTargetUuid)
	if targetObj.Has("buildPhases") {
		addToObjectList(targetObj, "buildPhases", CommentValue{
			Value:   buildPhaseUuid,
			Comment: comment,
		}.ToObject())
	}

	p.pbxBuildFileSection.ForeachWithFilter(func(key string, value interface{}) pegparser.IterateActionType {
		buildFileKey := fromCommentKey(key)
		buildFile := p.pbxBuildFileSection.GetObject(buildFileKey)
		fileReference := p.pbxFileReferenceSection.GetObject(buildFile.GetString("fileRef"))
		if fileReference.IsEmpty() {
			return pegparser.IterateActionContinue
		}
		filePath := fileReference.GetString("path")
		pbxFileObj := newPbxFile(filePath, newPbxFileOptions())
		filePathToBuildFile[filePath] = &PbxFile{
			Uuid:     buildFileKey,
			Basename: pbxFileObj.Basename,
			Group:    pbxFileObj.Group,
		}
		return pegparser.IterateActionContinue
	}, onlyCommentsFilter)

	for _, filePath := range filePathsArray {
		filePathQuoted := `"` + filePath + `"`
		pbxfile, ok := filePathToBuildFile[filePath]
		if ok {
			addToObjectList(buildPhase, "files", pbxBuildPhaseObj(pbxfile))
			continue
		}
		pbxfile, ok = filePathToBuildFile[filePathQuoted]
		if ok {
			addToObjectList(buildPhase, "files", pbxBuildPhaseObj(pbxfile))
			continue
		}

		pbxfile = newPbxFile(filePath, newPbxFileOptions())
		pbxfile.Uuid = p.generateUuid()
		pbxfile.FileRef = p.generateUuid()
		p.addToPbxFileReferenceSection(pbxfile) // PBXFileReference
		p.addToPbxBuildFileSection(pbxfile)     // PBXBuildFile
		addToObjectList(buildPhase, "files", pbxBuildPhaseObj(pbxfile))
	}
	buildPhaseSection.Set(buildPhaseUuid, buildPhase)
	buildPhaseSection.Set(commentKey, comment)
}

func (p *PbxProject) pbxGroupByName(name string) (obj pegparser.Object) {
	obj = pegparser.NewObject()
	p.pbxGroupSection.ForeachWithFilter(func(key string, value interface{}) pegparser.IterateActionType {
		if value.(string) == name {
			obj = p.pbxGroupSection.GetObject(fromCommentKey(key))
			return pegparser.IterateActionBreak
		}
		return pegparser.IterateActionContinue
	}, onlyCommentsFilter)
	return
}

func (p *PbxProject) pbxTargetByName(name string) pegparser.Object {
	return p.pbxItemByComment(name, "PBXNativeTarget")
}

func (p *PbxProject) findTargetKey(name string) (targetKey string) {
	targets := p.pbxObjectSection.GetObject("PBXNativeTarget")
	targets.ForeachWithFilter(func(key string, value interface{}) pegparser.IterateActionType {
		if value.(pegparser.Object).GetString("name") == name {
			targetKey = key
			return pegparser.IterateActionBreak
		}
		return pegparser.IterateActionContinue
	}, onlyCommentsFilter)
	return
}

func (p *PbxProject) pbxItemByComment(name, pbxSectionName string) (obj pegparser.Object) {
	obj = pegparser.NewObject()
	section := p.pbxObjectSection.GetObject(pbxSectionName)
	section.ForeachWithFilter(func(key string, value interface{}) pegparser.IterateActionType {
		if value.(string) == name {
			obj = section.GetObject(fromCommentKey(key))
			return pegparser.IterateActionBreak
		}
		return pegparser.IterateActionContinue
	}, onlyCommentsFilter)
	return
}

func (p *PbxProject) pbxSourcesBuildPhaseObj(target string) pegparser.Object {
	return p.buildPhaseObject("PBXSourcesBuildPhase", "Sources", target)
}

func (p *PbxProject) pbxResourcesBuildPhaseObj(target string) pegparser.Object {
	return p.buildPhaseObject("PBXResourcesBuildPhase", "Resources", target)
}

func (p *PbxProject) pbxFrameworksBuildPhaseObj(target string) pegparser.Object {
	return p.buildPhaseObject("PBXFrameworksBuildPhase", "Frameworks", target)
}

func (p *PbxProject) pbxEmbedFrameworksBuildPhaseObj(target string) pegparser.Object {
	return p.buildPhaseObject("PBXCopyFilesBuildPhase", "Embed Frameworks", target)
}

// Find Build Phase from group/target
func (p *PbxProject) buildPhase(group, target string) string {
	if target == "" {
		return ""
	}

	nativeTarget := p.pbxNativeTargetSection.GetObject(target)
	if nativeTarget.IsEmpty() {
		return ""
	}

	buildPhases := nativeTarget.ForceGet("buildPhases")
	if buildPhases == nil {
		return ""
	}

	for _, buildPhase := range buildPhases.([]interface{}) {
		if buildPhase.(pegparser.Object).GetString("comment") == group {
			return toCommentKey(buildPhase.(pegparser.Object).GetString("value"))
		}
	}

	return ""
}

func (p *PbxProject) buildPhaseObject(name, group, target string) (obj pegparser.Object) {
	obj = pegparser.NewObject()
	section := p.pbxObjectSection.GetObject(name)
	if section.IsEmpty() {
		return
	}
	buildPhase := p.buildPhase(group, target)
	section.ForeachWithFilter(func(key string, value interface{}) pegparser.IterateActionType {
		// select the proper buildPhase
		if buildPhase != "" && buildPhase != key {
			return pegparser.IterateActionContinue
		}

		if value.(string) == group {
			obj = section.GetObject(fromCommentKey(key))
			return pegparser.IterateActionBreak
		}
		return pegparser.IterateActionContinue
	}, onlyCommentsFilter)
	return
}

func (p *PbxProject) AddBuildProperty(prop, value, build_name string) {
	p.pbxXCBuildConfigurationSection.ForeachWithFilter(func(key string, val interface{}) pegparser.IterateActionType {
		configuration := val.(pegparser.Object)
		if build_name == "" || configuration.GetString("name") == build_name {
			configuration.GetObject("buildSettings").Set(prop, value)
		}
		return pegparser.IterateActionContinue
	}, nonCommentsFilter)
}

func (p *PbxProject) RemoveBuildProperty(prop, build_name string) {
	p.pbxXCBuildConfigurationSection.ForeachWithFilter(func(key string, val interface{}) pegparser.IterateActionType {
		configuration := val.(pegparser.Object)
		if build_name == "" || configuration.GetString("name") == build_name {
			configuration.GetObject("buildSettings").Delete(prop)
		}
		return pegparser.IterateActionContinue
	}, nonCommentsFilter)
}

func (p *PbxProject) UpdateBuildProperty(prop, value, build, targetName string) {
	validConfigs := make(map[string]struct{})
	if targetName != "" {
		target := p.pbxTargetByName(targetName)
		if !target.IsEmpty() {
			targetBuildConfigs := target.GetString("buildConfigurationList")
			p.pbxXCConfigurationListSection.ForeachWithFilter(func(configName string, val interface{}) pegparser.IterateActionType {
				if targetBuildConfigs == configName {
					buildVariants := val.(pegparser.Object).ForceGet("buildConfigurations")
					for _, buildVariant := range buildVariants.([]interface{}) {
						validConfigs[buildVariant.(pegparser.Object).GetString("value")] = struct{}{}
					}
					return pegparser.IterateActionBreak
				}
				return pegparser.IterateActionContinue
			}, nonCommentsFilter)
		}
	}

	p.pbxXCConfigurationListSection.ForeachWithFilter(func(configName string, val interface{}) pegparser.IterateActionType {
		if targetName != "" {
			_, found := validConfigs[configName]
			if !found {
				return pegparser.IterateActionContinue
			}
		}

		if build == "" || val.(pegparser.Object).GetString("name") == build {
			val.(pegparser.Object).Set(prop, value)
		}
		return pegparser.IterateActionContinue
	}, nonCommentsFilter)
}

func (p *PbxProject) UpdateProductName(name string) {
	p.UpdateBuildProperty("PRODUCT_NAME", `"`+name+`"`, "", "")
}

func (p *PbxProject) addToSearchPaths(searchPath string, pbxfile *PbxFile) {
	INHERITED := `"$(inherited)"`
	p.pbxXCBuildConfigurationSection.ForeachWithFilter(func(key string, val interface{}) pegparser.IterateActionType {
		buildSettings := val.(pegparser.Object).GetObject("buildSettings")
		if unquoted(buildSettings.GetString("PRODUCT_NAME")) != p.productName() {
			return pegparser.IterateActionContinue
		}
		searchPathsInterface := buildSettings.ForceGet(searchPath)
		searchPathsStr, ok := searchPathsInterface.(string)
		if ok && searchPathsStr == INHERITED {
			buildSettings.Set(searchPath, []interface{}{INHERITED})
		}

		addToObjectList(buildSettings, searchPath, p.searchPathForFile(pbxfile))
		return pegparser.IterateActionContinue
	}, nonCommentsFilter)
}

func (p *PbxProject) removeFromSearchPaths(searchPath string, pbxfile *PbxFile) {
	new_path := p.searchPathForFile(pbxfile)
	p.pbxXCBuildConfigurationSection.ForeachWithFilter(func(key string, val interface{}) pegparser.IterateActionType {
		buildSettings := val.(pegparser.Object).GetObject("buildSettings")
		if unquoted(buildSettings.GetString("PRODUCT_NAME")) != p.productName() {
			return pegparser.IterateActionContinue
		}

		addToObjectListOnlyNotExist(buildSettings, searchPath, new_path, func(v1, v2 interface{}) bool {
			return v1.(string) == v2.(string)
		})

		return pegparser.IterateActionContinue
	}, nonCommentsFilter)
}

func (p *PbxProject) addToFrameworkSearchPaths(pbxfile *PbxFile) {
	p.addToSearchPaths("FRAMEWORK_SEARCH_PATHS", pbxfile)
}

func (p *PbxProject) removeFromFrameworkSearchPaths(pbxfile *PbxFile) {
	p.removeFromSearchPaths("FRAMEWORK_SEARCH_PATHS", pbxfile)
}

func (p *PbxProject) addToLibrarySearchPaths(pbxfile *PbxFile) {
	p.addToSearchPaths("LIBRARY_SEARCH_PATHS", pbxfile)
}

func (p *PbxProject) removeFromLibrarySearchPaths(pbxfile *PbxFile) {
	p.removeFromSearchPaths("LIBRARY_SEARCH_PATHS", pbxfile)
}

func (p *PbxProject) addToHeaderSearchPaths(pbxfile *PbxFile) {
	p.addToSearchPaths("HEADER_SEARCH_PATHS", pbxfile)
}

func (p *PbxProject) removeFromHeaderSearchPaths(pbxfile *PbxFile) {
	p.removeFromSearchPaths("HEADER_SEARCH_PATHS", pbxfile)
}

func (p *PbxProject) addToOtherLinkerFlags(pbxfile *PbxFile) {
	p.addToSearchPaths("OTHER_LDFLAGS", pbxfile)
}

func (p *PbxProject) removeFromOtherLinkerFlags(pbxfile *PbxFile) {
	p.removeFromSearchPaths("OTHER_LDFLAGS", pbxfile)
}

func (p *PbxProject) addToBuildSettings(buildSetting string, value interface{}) {
	p.pbxXCBuildConfigurationSection.ForeachWithFilter(func(key string, val interface{}) pegparser.IterateActionType {
		buildSettings := val.(pegparser.Object).GetObject("buildSettings")
		buildSettings.Set(buildSetting, value)
		return pegparser.IterateActionContinue
	}, nonCommentsFilter)
}

func (p *PbxProject) removeFromBuildSettings(buildSetting string) {
	p.pbxXCBuildConfigurationSection.ForeachWithFilter(func(key string, val interface{}) pegparser.IterateActionType {
		buildSettings := val.(pegparser.Object).GetObject("buildSettings")
		buildSettings.Delete(buildSetting)
		return pegparser.IterateActionContinue
	}, nonCommentsFilter)
}

// // a JS getter. hmmm
func (p *PbxProject) productName() (name string) {
	p.pbxXCBuildConfigurationSection.ForeachWithFilter(func(key string, val interface{}) pegparser.IterateActionType {
		buildSettings := val.(pegparser.Object).GetObject("buildSettings")
		productName := buildSettings.GetString("PRODUCT_NAME")
		if productName != "" {
			name = unquoted(productName)
			return pegparser.IterateActionBreak
		}
		return pegparser.IterateActionContinue
	}, nonCommentsFilter)
	return
}

// // check if file is present
func (p *PbxProject) getFile(filePath string) *PbxFile {
	pbxfile, ok := p.pbxFileNoCommentReferences[filePath]
	if ok {
		return pbxfile
	}
	pbxfile, ok = p.pbxFileNoCommentReferences[`"`+filePath+`"`]
	if ok {
		return pbxfile
	}

	return nil
}

func (p *PbxProject) hasFile(filePath string) bool {
	return p.getFile(filePath) != nil
}

func (p *PbxProject) AddTarget(name, targetType, subfolder, bundleId string) error {
	// Setup uuid and name of new target
	targetUuid := p.generateUuid()
	targetSubfolder := subfolder
	if targetSubfolder == "" {
		targetSubfolder = name
	}
	targetName := strings.Trim(name, " ")
	targetBundleId := bundleId

	// Check type against list of allowed target types
	if targetName == "" {
		return fmt.Errorf("Target name missing.")
	}

	// Check type against list of allowed target types
	if targetType == "" {
		return fmt.Errorf("Target type missing.")
	}

	// Check type against list of allowed target types
	if producttypeForTargettype(targetType) == "" {
		return fmt.Errorf("Target type invalid: %s\n", targetType)
	}

	// Build Configuration: Create
	buildConfigurationsList := []pegparser.Object{
		pegparser.NewObjectWithData([]pegparser.SliceItem{
			pegparser.NewObjectItem("name", "Debug"),
			pegparser.NewObjectItem("isa", "XCBuildConfiguration"),
			pegparser.NewObjectItem("buildSettings", pegparser.NewObjectWithData([]pegparser.SliceItem{
				pegparser.NewObjectItem("GCC_PREPROCESSOR_DEFINITIONS", []string{`"DEBUG=1"`, `"$(inherited)"`}),
				pegparser.NewObjectItem("INFOPLIST_FILE", `"`+filepath.Join(targetSubfolder, targetSubfolder+"-Info.plist"+`"`)),
				pegparser.NewObjectItem("LD_RUNPATH_SEARCH_PATHS", `"$(inherited) @executable_path/Frameworks @executable_path/../../Frameworks"`),
				pegparser.NewObjectItem("PRODUCT_NAME", `"`+targetName+`"`),
				pegparser.NewObjectItem("SKIP_INSTALL", "YES"),
			})),
		}),
		pegparser.NewObjectWithData([]pegparser.SliceItem{
			pegparser.NewObjectItem("name", "Release"),
			pegparser.NewObjectItem("isa", "XCBuildConfiguration"),
			pegparser.NewObjectItem("buildSettings", pegparser.NewObjectWithData([]pegparser.SliceItem{
				pegparser.NewObjectItem("INFOPLIST_FILE", `"`+filepath.Join(targetSubfolder, targetSubfolder+"-Info.plist"+`"`)),
				pegparser.NewObjectItem("LD_RUNPATH_SEARCH_PATHS", `"$(inherited) @executable_path/Frameworks @executable_path/../../Frameworks"`),
				pegparser.NewObjectItem("PRODUCT_NAME", `"`+targetName+`"`),
				pegparser.NewObjectItem("SKIP_INSTALL", "YES"),
			})),
		}),
	}

	// Add optional bundleId to build configuration
	if targetBundleId != "" {
		for _, buildConfiguration := range buildConfigurationsList {
			buildConfiguration.GetObject("buildSettings").Set("PRODUCT_BUNDLE_IDENTIFIER", `"`+targetBundleId+`"`)
		}
	}

	// Build Configuration: Add
	buildConfigurations := p.addXCConfigurationList(buildConfigurationsList, "Release", `Build configuration list for PBXNativeTarget "`+targetName+`"`)

	// Product: Create
	productName := targetName
	productType := producttypeForTargettype(targetType)
	productFileType := filetypeForProducttype(productType)
	productFile := p.addProductFile(productName, PbxFileOptions{
		Group:            "Copy Files",
		Target:           targetUuid,
		ExplicitFileType: productFileType,
	})

	// Product: Add to build file list
	p.addToPbxBuildFileSection(productFile)

	// Target: Create
	target := pegparser.NewObjectWithData([]pegparser.SliceItem{
		pegparser.NewObjectItem("isa", "PBXNativeTarget"),
		pegparser.NewObjectItem("name", `"`+targetName+`"`),
		pegparser.NewObjectItem("productName", `"`+targetName+`"`),
		pegparser.NewObjectItem("productReference", productFile.FileRef),
		pegparser.NewObjectItem("productType", `"`+producttypeForTargettype(targetType)+`"`),
		pegparser.NewObjectItem("buildConfigurationList", buildConfigurations.UUID),
		pegparser.NewObjectItem("buildPhases", []interface{}{}),
		pegparser.NewObjectItem("buildRules", []interface{}{}),
		pegparser.NewObjectItem("dependencies", []interface{}{}),
	})

	// Target: Add to PBXNativeTarget section
	p.addToPbxNativeTargetSection(targetUuid, target)

	// Product: Embed (only for "extension"-type targets)
	if targetType == "app_extension" {

		// Create CopyFiles phase in first target
		p.AddBuildPhase([]string{}, "PBXCopyFilesBuildPhase", "Copy Files", p.getFirstTarget().UUID, targetType, "")

		// Add product to CopyFiles phase
		p.addToPbxCopyfilesBuildPhase(productFile)

		// this.addBuildPhaseToTarget(newPhase.buildPhase, this.getFirstTarget().uuid)
	} else if targetType == "watch2_app" {
		// Create CopyFiles phase in first target
		p.AddBuildPhase(
			[]string{targetName + ".app"},
			"PBXCopyFilesBuildPhase",
			"Embed Watch Content",
			p.getFirstTarget().UUID,
			targetType,
			`"$(CONTENTS_FOLDER_PATH)/Watch"`,
		)
	} else if targetType == "watch2_extension" {
		// Create CopyFiles phase in watch target (if exists)
		watch2Target := p.getTarget(producttypeForTargettype("watch2_app"))
		if watch2Target.UUID != "" {
			p.AddBuildPhase(
				[]string{targetName + ".appex"},
				"PBXCopyFilesBuildPhase",
				"Embed App Extensions",
				watch2Target.UUID,
				targetType,
				"",
			)
		}
	}

	// Target: Add uuid to root project
	p.addToPbxProjectSection(targetUuid, target)

	// Target: Add dependency for this target to other targets
	if targetType == "watch2_extension" {
		watch2Target := p.getTarget(producttypeForTargettype("watch2_app"))
		if watch2Target.UUID != "" {
			p.AddTargetDependency(watch2Target.UUID, []string{targetUuid})
		}
	} else {
		p.AddTargetDependency(p.getFirstTarget().UUID, []string{targetUuid})
	}

	return nil
}

// // helper object creation functions
func pbxBuildFileObj(pbxfile *PbxFile) pegparser.Object {
	obj := pegparser.NewObject()
	obj.Set("isa", "PBXBuildFile")
	obj.Set("fileRef", pbxfile.FileRef)
	obj.Set(toCommentKey("fileRef"), pbxfile.Basename)
	if !pbxfile.Settings.IsEmpty() {
		obj.Set("settings", pbxfile.Settings)
	}
	return obj
}

func newPbxFileReferenceObj(pbxfile *PbxFile) pegparser.Object {
	return pegparser.NewObjectWithData([]pegparser.SliceItem{
		pegparser.NewObjectItem("isa", "PBXFileReference"),
		pegparser.NewObjectItem("name", `"`+pbxfile.Basename+`"`),
		pegparser.NewObjectItem("fileEncoding", pbxfile.FileEncoding),
		pegparser.NewObjectItem("lastKnownFileType", pbxfile.LastKnownFileType),
		pegparser.NewObjectItem("path", `"`+filepath.ToSlash(pbxfile.Path)+`"`),
		pegparser.NewObjectItem("sourceTree", pbxfile.SourceTree),
		pegparser.NewObjectItem("explicitFileType", pbxfile.ExplicitFileType),
		pegparser.NewObjectItem("includeInIndex", pbxfile.IncludeInIndex),
	})
}

func pbxGroupChild(pbxfile *PbxFile) CommentValue {
	return CommentValue{
		Value:   pbxfile.FileRef,
		Comment: pbxfile.Basename,
	}
}

func pbxBuildPhaseObj(pbxfile *PbxFile) pegparser.Object {
	obj := pegparser.NewObject()
	obj.Set("value", pbxfile.Uuid)
	obj.Set("comment", longComment(pbxfile))
	return obj
}

func pbxCopyFilesBuildPhaseObj(obj pegparser.Object, folderType, subfolderPath, phaseName string) pegparser.Object {

	// Add additional properties for "CopyFiles" build phase
	DESTINATION_BY_TARGETTYPE := map[string]string{
		"application":       "wrapper",
		"app_extension":     "plugins",
		"bundle":            "wrapper",
		"command_line_tool": "wrapper",
		"dynamic_library":   "products_directory",
		"framework":         "shared_frameworks",
		"frameworks":        "frameworks",
		"static_library":    "products_directory",
		"unit_test_bundle":  "wrapper",
		"watch_app":         "wrapper",
		"watch2_app":        "products_directory",
		"watch_extension":   "plugins",
		"watch2_extension":  "plugins",
	}
	SUBFOLDERSPEC_BY_DESTINATION := map[string]int{
		"absolute_path":      0,
		"executables":        6,
		"frameworks":         10,
		"java_resources":     15,
		"plugins":            13,
		"products_directory": 16,
		"resources":          7,
		"shared_frameworks":  11,
		"shared_support":     12,
		"wrapper":            1,
		"xpc_services":       0,
	}

	obj.Set("name", `"`+phaseName+`"`)

	if subfolderPath == "" {
		subfolderPath = `""`
	}
	obj.Set("dstPath", subfolderPath)
	obj.Set("dstSubfolderSpec", SUBFOLDERSPEC_BY_DESTINATION[DESTINATION_BY_TARGETTYPE[folderType]])
	return obj
}

type pbxShellScriptBuildPhaseObjOptions struct {
	InputPaths  []string
	OutputPaths []string
	ShellScript string
}

func pbxShellScriptBuildPhaseObj(obj pegparser.Object, options pbxShellScriptBuildPhaseObjOptions, phaseName string) pegparser.Object {
	obj.Set("name", `"`+phaseName+`"`)
	if options.InputPaths != nil {
		obj.Set("inputPaths", options.InputPaths)
	} else {
		obj.Set("inputPaths", []interface{}{})
	}

	if options.InputPaths != nil {
		obj.Set("outputPaths", options.InputPaths)
	} else {
		obj.Set("outputPaths", []interface{}{})
	}
	obj.Set("shellPath", options.ShellScript)
	obj.Set("shellScript", `"`+strings.ReplaceAll(options.ShellScript, `"`, `\\"`)+`"`)
	return obj
}

func pbxBuildFileComment(pbxfile *PbxFile) string {
	return longComment(pbxfile)
}

func pbxFileReferenceComment(pbxfile *PbxFile) string {
	if pbxfile.Basename != "" {
		return pbxfile.Basename
	} else {
		return filepath.Base(pbxfile.Path)
	}
}

func pbxNativeTargetComment(target pegparser.Object) string {
	return target.GetString("name")
}

func longComment(pbxfile *PbxFile) string {
	return fmt.Sprintf("%s in %s", pbxfile.Basename, pbxfile.Group)
}

// respect <group> path
func (p *PbxProject) correctForPluginsPath(pbxFile *PbxFile) *PbxFile {
	return p.correctForPath(pbxFile, "Plugins")
}

func (p *PbxProject) correctForResourcesPath(pbxFile *PbxFile) *PbxFile {
	return p.correctForPath(pbxFile, "Resources")
}

func (p *PbxProject) correctForFrameworksPath(pbxFile *PbxFile) *PbxFile {
	return p.correctForPath(pbxFile, "Frameworks")
}

func (p *PbxProject) correctForPath(pbxFile *PbxFile, groupName string) *PbxFile {
	r_group_dir := regexp.MustCompile("^" + groupName + "[\\\\/]")

	group := p.pbxGroupByName(groupName)
	if !group.IsEmpty() {
		if group.GetString("path") != "" {
			pbxFile.Path = r_group_dir.ReplaceAllString(pbxFile.Path, "")
		}
	}

	return pbxFile
}

func (p *PbxProject) searchPathForFile(pdxfile *PbxFile) string {
	pluginsPath := ""
	plugins := p.pbxGroupByName("Plugins")
	if !plugins.IsEmpty() {
		pluginsPath = plugins.GetString("path")
	}

	fileDir := filepath.Dir(pdxfile.Path)
	if fileDir == "." {
		fileDir = ""
	} else {
		fileDir = "/" + fileDir
	}

	if pdxfile.Plugin && pluginsPath != "" {
		return `"\"$(SRCROOT)/` + unquoted(pluginsPath) + `\""`
	} else if pdxfile.CustomFramework && pdxfile.Dirname != "" {
		return `"\"` + pdxfile.Dirname + `\""`
	} else {
		return `"\"$(SRCROOT)/` + p.productName() + fileDir + `\""`
	}
}

func buildPhaseNameForIsa(isa string) string {
	switch isa {
	case "PBXCopyFilesBuildPhase":
		return "Copy Files"
	case "PBXResourcesBuildPhase":
		return "Resources"
	case "PBXSourcesBuildPhase":
		return "Sources"
	case "PBXFrameworksBuildPhase":
		return "Frameworks"
	default:
		return ""
	}
}

func producttypeForTargettype(targetType string) string {

	switch targetType {
	case "application":
		return "com.apple.product-type.application"
	case "app_extension":
		return "com.apple.product-type.app-extension"
	case "bundle":
		return "com.apple.product-type.bundle"
	case "command_line_tool":
		return "com.apple.product-type.tool"
	case "dynamic_library":
		return "com.apple.product-type.library.dynamic"
	case "framework":
		return "com.apple.product-type.framework"
	case "static_library":
		return "com.apple.product-type.library.static"
	case "unit_test_bundle":
		return "com.apple.product-type.bundle.unit-test"
	case "watch_app":
		return "com.apple.product-type.application.watchapp"
	case "watch2_app":
		return "com.apple.product-type.application.watchapp2"
	case "watch_extension":
		return "com.apple.product-type.watchkit-extension"
	case "watch2_extension":
		return "com.apple.product-type.watchkit2-extension"
	default:
		return ""
	}
}

func filetypeForProducttype(productType string) string {

	switch productType {
	case "com.apple.product-type.application":
		return "wrapper.application"
	case "com.apple.product-type.app-extension":
		return "wrapper.app-extension"
	case "com.apple.product-type.bundle":
		return "wrapper.plug-in"
	case "com.apple.product-type.tool":
		return "compiled.mach-o.dylib"
	case "com.apple.product-type.library.dynamic":
		return "compiled.mach-o.dylib"
	case "com.apple.product-type.framework":
		return "wrapper.framework"
	case "com.apple.product-type.library.static":
		return "archive.ar"
	case "com.apple.product-type.bundle.unit-test":
		return "wrapper.cfbundle"
	case "com.apple.product-type.application.watchapp":
		return "wrapper.application"
	case "com.apple.product-type.application.watchapp2":
		return "wrapper.application"
	case "com.apple.product-type.watchkit-extension":
		return "wrapper.app-extension"
	case "com.apple.product-type.watchkit2-extension":
		return "wrapper.app-extension"
	default:
		return ""
	}
}

func (p *PbxProject) getFirstProject() pegparser.ObjectWithUUID {
	uuid := ""
	var project pegparser.Object
	p.pbxProjectSection.ForeachWithFilter(func(key string, value interface{}) pegparser.IterateActionType {
		uuid = key
		project = value.(pegparser.Object)
		return pegparser.IterateActionBreak
	}, nonCommentsFilter)

	return pegparser.ObjectWithUUID{
		UUID:   uuid,
		Object: project,
	}
}

func (p *PbxProject) getFirstTarget() pegparser.ObjectWithUUID {
	project := p.getFirstProject()
	firstTargetUuid := project.Object.ForceGet("targets").([]interface{})[0].(pegparser.Object).GetString("value")
	firstTarget := p.pbxNativeTargetSection.GetObject(firstTargetUuid)

	return pegparser.ObjectWithUUID{
		UUID:   firstTargetUuid,
		Object: firstTarget,
	}
}

func (p *PbxProject) getTarget(productType string) (targetWithUUID pegparser.ObjectWithUUID) {
	project := p.getFirstProject()
	targets := project.Object.GetObject("targets")

	targets.Foreach(func(key string, value interface{}) pegparser.IterateActionType {
		targetUUID := value.(pegparser.Object).GetString("value")
		target := p.pbxNativeTargetSection.GetObject(targetUUID)
		if target.GetString("productType") == `"`+productType+`"` {
			targetWithUUID = pegparser.ObjectWithUUID{
				UUID:   targetUUID,
				Object: target,
			}
			return pegparser.IterateActionBreak
		}
		return pegparser.IterateActionContinue
	})

	return
}

func (p *PbxProject) addToPbxGroupType(childGroup CommentValue, groupKey, groupType string) {
	group := p.getPBXGroupByKeyAndType(groupKey, groupType)
	if group.IsEmpty() {
		return
	}
	children := group.ForceGet("children")
	if children == nil {
		return
	}
	children = append(children.([]interface{}), childGroup)
	group.Set("children", children)
}

func (p *PbxProject) addToPbxVariantGroup(pbxfile *PbxFile, groupKey string) {
	p.addToPbxGroupType(pbxGroupChild(pbxfile), groupKey, "PBXVariantGroup")
}

func (p *PbxProject) addToPbxGroupByKey(pbxfile *PbxFile, groupKey string) {
	p.addToPbxGroupType(pbxGroupChild(pbxfile), groupKey, "PBXGroup")
}

func (p *PbxProject) pbxCreateGroupWithType(name, pathName, groupType string) string {
	//Create object
	model := pegparser.NewObjectWithData([]pegparser.SliceItem{
		pegparser.NewObjectItem("isa", `"`+groupType+`"`),
		pegparser.NewObjectItem("children", []interface{}{}),
		pegparser.NewObjectItem("name", name),
		pegparser.NewObjectItem("sourceTree", `"<group>"`),
	})

	if pathName != "" {
		model.Set("path", pathName)
	}
	key := p.generateUuid()

	//add obj and commentObj to groups;
	group := p.pbxGroupSection.GetObject(groupType)
	if group.IsEmpty() {
		group = pegparser.NewObject()
		p.pbxGroupSection.Set(groupType, group)
	}

	group.Set(key, model)
	group.Set(toCommentKey(key), name)
	return key
}

func (p *PbxProject) pbxCreateVariantGroup(name string) string {
	return p.pbxCreateGroupWithType(name, "", "PBXVariantGroup")
}

func (p *PbxProject) pbxCreateGroup(name, pathName string) string {
	return p.pbxCreateGroupWithType(name, pathName, "PBXGroup")
}

func (p *PbxProject) removeFromPbxGroupAndType(pbxfile *PbxFile, groupKey, groupType string) {
	group := p.getPBXGroupByKeyAndType(groupKey, groupType)
	if !group.IsEmpty() {
		child := pbxGroupChild(pbxfile)
		groupChildren := group.ForceGet("children").([]interface{})
		for i, v := range groupChildren {
			if child.Value == v.(pegparser.Object).GetString("value") && child.Comment == v.(pegparser.Object).GetString("comment") {
				groupChildren = append(groupChildren[:i], groupChildren[i+1:]...)
				group.Set("children", groupChildren)
				break
			}
		}
	}
}

func (p *PbxProject) removeFromPbxGroupByKey(pbxfile *PbxFile, groupKey string) {
	p.removeFromPbxGroupAndType(pbxfile, groupKey, "PBXGroup")
}

func (p *PbxProject) removeFromPbxVariantGroup(pbxfile *PbxFile, groupKey string) {
	p.removeFromPbxGroupAndType(pbxfile, groupKey, "PBXVariantGroup")
}

func (p *PbxProject) getPBXGroupByKeyAndType(key, groupType string) pegparser.Object {
	return p.pbxObjectSection.GetObject(groupType).GetObject(key)
}

func (p *PbxProject) getPBXGroupByKey(key string) pegparser.Object {
	return p.getPBXGroupByKeyAndType(key, "PBXGroup")
}

func (p *PbxProject) getPBXVariantGroupByKey(key string) pegparser.Object {
	return p.getPBXGroupByKeyAndType(key, "PBXVariantGroup")
}

type FindGroupCriteria struct {
	Name string
	Path string
}

func (p *PbxProject) findPBXGroupKeyAndType(criteria FindGroupCriteria, groupType string) (target string) {
	if criteria.Name == "" && criteria.Path == "" {
		return
	}

	groups := p.pbxObjectSection.GetObject(groupType)
	groups.ForeachWithFilter(func(key string, value interface{}) pegparser.IterateActionType {
		group := value.(pegparser.Object)
		if criteria.Name != "" && criteria.Name != group.GetString("name") {
			return pegparser.IterateActionContinue
		}

		if criteria.Path != "" && criteria.Path != group.GetString("path") {
			return pegparser.IterateActionContinue
		}

		target = key
		return pegparser.IterateActionBreak
	}, onlyCommentsFilter)
	return
}

func (p *PbxProject) findPBXGroupKey(criteria FindGroupCriteria) string {
	return p.findPBXGroupKeyAndType(criteria, "PBXGroup")
}

func (p *PbxProject) findPBXVariantGroupKey(criteria FindGroupCriteria) string {
	return p.findPBXGroupKeyAndType(criteria, "PBXVariantGroup")
}

func (p *PbxProject) AddLocalizationVariantGroup(name string) *PbxFile {
	groupKey := p.pbxCreateVariantGroup(name)
	resourceGroupKey := p.findPBXGroupKey(FindGroupCriteria{Name: "Resources"})

	childGroup := CommentValue{
		Value: groupKey,
	}
	if !p.getPBXGroupByKey(groupKey).IsEmpty() {
		childGroup.Comment = p.getPBXGroupByKey(groupKey).GetString("name")
	} else if !p.getPBXVariantGroupByKey(groupKey).IsEmpty() {
		childGroup.Comment = p.getPBXVariantGroupByKey(groupKey).GetString("name")
	}
	p.addToPbxGroupType(childGroup, resourceGroupKey, "PBXGroup")

	localizationVariantGroup := &PbxFile{
		Uuid:     p.generateUuid(),
		FileRef:  groupKey,
		Basename: name,
	}
	p.addToPbxBuildFileSection(localizationVariantGroup)    // PBXBuildFile
	p.addToPbxResourcesBuildPhase(localizationVariantGroup) //PBXResourcesBuildPhase
	return localizationVariantGroup
}

func (p *PbxProject) AddKnownRegion(name string) {
	firstProject := p.getFirstProject()
	if firstProject.UUID == "" {
		return
	}

	project := p.pbxProjectSection.GetObject(firstProject.GetString("project"))
	if project.IsEmpty() {
		return
	}
	if !project.Has("knownRegions") {
		project.Set("knownRegions", []string{name})
	} else if !p.HasKnownRegion(name) {
		knownRegions := project.ForceGet("knownRegions").([]interface{})
		knownRegions = append(knownRegions, name)
		project.Set("knownRegions", knownRegions)
	}
}

func (p *PbxProject) RemoveKnownRegion(name string) {
	firstProject := p.getFirstProject()
	if firstProject.UUID == "" {
		return
	}

	projectUuid := firstProject.GetString("project")
	project := p.pbxProjectSection.GetObject(projectUuid)
	if project.IsEmpty() {
		return
	}
	knownRegions := project.ForceGet("knownRegions")
	if knownRegions == nil {
		return
	}

	for i, v := range knownRegions.([]interface{}) {
		if v.(string) == name {
			knownRegions = append(knownRegions.([]interface{})[:i], knownRegions.([]interface{})[i+1:]...)
			project.Set("knownRegions", knownRegions)
			break
		}
	}
}

func (p *PbxProject) HasKnownRegion(name string) bool {
	firstProject := p.getFirstProject()
	if firstProject.UUID == "" {
		return false
	}

	projectUuid := firstProject.GetString("project")
	project := p.pbxProjectSection.GetObject(projectUuid)
	if project.IsEmpty() {
		return false
	}
	knownRegions := project.ForceGet("knownRegions")
	if knownRegions == nil {
		return false
	}

	for _, v := range knownRegions.([]interface{}) {
		if v.(string) == name {
			return true
		}
	}

	return false
}

func (p *PbxProject) getPBXObject(name string) pegparser.Object {
	return p.pbxObjectSection.GetObject(name)
}

func (p *PbxProject) addFile(path, group string, opts PbxFileOptions) (*PbxFile, error) {
	pbxfile := newPbxFile(path, opts)
	if p.hasFile(pbxfile.Path) {
		return nil, fmt.Errorf("file %s already exists", pbxfile.Path)
	}

	pbxfile.FileRef = p.generateUuid()
	p.addToPbxFileReferenceSection(pbxfile) // PBXFileReference
	if !p.getPBXGroupByKey(group).IsEmpty() {
		p.addToPbxGroup(pbxfile, group) // PBXGroup
	} else if !p.getPBXVariantGroupByKey(group).IsEmpty() {
		p.addToPbxVariantGroup(pbxfile, group) // PBXVariantGroup
	}

	return pbxfile, nil
}

func (p *PbxProject) AddFile(filePath string, params ...interface{}) error {
	options, group := parseFileVariadicParams(params...)
	_, err := p.addFile(filePath, group, options)
	return err
}

func (p *PbxProject) removeFile(path, group string, opt PbxFileOptions) *PbxFile {
	pbxfile := newPbxFile(path, opt)

	p.removeFromPbxFileReferenceSection(pbxfile) // PBXFileReference

	if !p.getPBXGroupByKey(group).IsEmpty() {
		p.removeFromPbxGroup(pbxfile, group) // PBXGroup
	} else if !p.getPBXVariantGroupByKey(group).IsEmpty() {
		p.removeFromPbxVariantGroup(pbxfile, group) // PBXVariantGroup
	}

	return pbxfile
}
func (p *PbxProject) RemoveFile(filePath string, params ...interface{}) error {
	options, group := parseFileVariadicParams(params...)
	p.removeFile(filePath, group, options)
	return nil
}

func (p *PbxProject) GetBuildProperty(prop, build, targetName string) (props []string) {
	validConfigs := make(map[string]struct{})
	if targetName != "" {
		target := p.pbxTargetByName(targetName)
		if !target.IsEmpty() {
			targetBuildConfigs := target.GetString("buildConfigurationList")
			p.pbxXCConfigurationListSection.ForeachWithFilter(func(configName string, val interface{}) pegparser.IterateActionType {
				if targetBuildConfigs == configName {
					buildVariants := val.(pegparser.Object).ForceGet("buildConfigurations")
					for _, buildVariant := range buildVariants.([]interface{}) {
						validConfigs[buildVariant.(pegparser.Object).GetString("value")] = struct{}{}
					}
					return pegparser.IterateActionBreak
				}
				return pegparser.IterateActionContinue
			}, nonCommentsFilter)
		}
	}

	p.pbxXCBuildConfigurationSection.ForeachWithFilter(func(configName string, val interface{}) pegparser.IterateActionType {
		if targetName != "" {
			_, found := validConfigs[configName]
			if !found {
				return pegparser.IterateActionContinue
			}
		}

		if build == "" || val.(pegparser.Object).ForceGet("name") == build {
			props = interfaceToStringSlice(val.(pegparser.Object).ForceGet(prop))
			return pegparser.IterateActionBreak
		}
		return pegparser.IterateActionContinue
	}, nonCommentsFilter)
	return
}

func (p *PbxProject) getBuildConfigByName(name string) map[string]pegparser.Object {
	targets := map[string]pegparser.Object{}
	p.pbxXCBuildConfigurationSection.ForeachWithFilter(func(configName string, val interface{}) pegparser.IterateActionType {
		if val.(pegparser.Object).GetString("name") == name {
			targets[configName] = val.(pegparser.Object)
		}
		return pegparser.IterateActionContinue
	}, nonCommentsFilter)

	return targets
}

// func (p *PbxProject) addDataModelDocument(filePath, group string, opt PbxPbxFileOptions) *PbxFile {
// 	if group == "" {
// 		group = "Resources"
// 	}

//     if p.getPBXGroupByKey(group) == nil {
//         group = p.findPBXGroupKey(FindGroupCriteria{ Name: group })
//     }

//     pbxfile := newPbxFile(filePath, opt)
//     if p.hasFile(pbxfile.Path) {
// 		return nil
// 	}

//     pbxfile.FileRef = p.generateUuid()
//     p.addToPbxGroup(pbxfile, group)

//     pbxfile.Target = opt.Target
//     pbxfile.Uuid = p.generateUuid()

//     p.addToPbxBuildFileSection(pbxfile)
//     p.addToPbxSourcesBuildPhase(pbxfile)

//     pbxfile.Models = []
//     var currentVersionName;
// 	var modelFiles map[string]string
//     modelFilesData, err := ioutil.ReadAll(pbxfile.Path)
// 	if err != nil {
// 		return nil
// 	}
// 	err = json.Unmarshal(modelFilesData, &modelFiles)
// 	if err != nil {
// 		return nil
// 	}

// 	for modelName, modelFileName := range modelFiles {
//         modelFilePath := filepath.Join(filePath, modelFileName)

//         if modelFileName == ".xccurrentversion" {
// 			plist.NewD
//             currentVersionName = plist.readFileSync(modelFilePath)._XCCurrentVersionName;
//             continue;
//         }

//         var modelFile = new pbxFile(modelFilePath);
//         modelFile.fileRef = this.generateUuid();

//         this.addToPbxFileReferenceSection(modelFile);

//         file.models.push(modelFile);

//         if (currentVersionName && currentVersionName === modelFileName) {
//             file.currentModel = modelFile;
//         }

// 	}

//     for (var index in modelFiles) {
//         var modelFileName = modelFiles[index];
//         var modelFilePath = path.join(filePath, modelFileName);

//         if (modelFileName == ".xccurrentversion") {
//             currentVersionName = plist.readFileSync(modelFilePath)._XCCurrentVersionName;
//             continue;
//         }

//         var modelFile = new pbxFile(modelFilePath);
//         modelFile.fileRef = this.generateUuid();

//         this.addToPbxFileReferenceSection(modelFile);

//         file.models.push(modelFile);

//         if (currentVersionName && currentVersionName === modelFileName) {
//             file.currentModel = modelFile;
//         }
//     }

//     if (!file.currentModel) {
//         file.currentModel = file.models[0];
//     }

//     this.addToXcVersionGroupSection(file);

//     return file;
// }

func (p *PbxProject) AddTargetAttribute(prop, value string, target pegparser.ObjectWithUUID) error {
	project := p.getFirstProject()
	if project.UUID == "" {
		return errors.New("No project found")
	}
	attributes := project.Object.GetObject("attributes")
	if attributes.IsEmpty() {
		return errors.New("No attributes found")
	}

	targetAttrs := attributes.GetObject("TargetAttributes")
	if targetAttrs.IsEmpty() {
		targetAttrs = pegparser.NewObject()
		attributes.Set("TargetAttributes", targetAttrs)
	}

	if target.UUID == "" {
		target = p.getFirstTarget()
		if target.UUID == "" {
			return errors.New("No target found")
		}
	}

	targetAttr := targetAttrs.GetObject(target.UUID)
	if !targetAttr.IsEmpty() {
		targetAttr := pegparser.NewObject()
		attributes.Set(target.UUID, targetAttr)
	}
	targetAttr.Set(prop, value)
	return nil
}

func (p *PbxProject) RemoveTargetAttribute(prop string, target pegparser.ObjectWithUUID) error {
	project := p.getFirstProject()
	if project.UUID == "" {
		return errors.New("No project found")
	}
	attributes := project.Object.GetObject("attributes")
	if attributes.IsEmpty() {
		return errors.New("No attributes found")
	}
	if target.UUID == "" {
		target = p.getFirstTarget()
		if target.UUID == "" {
			return errors.New("No target found")
		}
	}

	targetAttrs := attributes.GetObject("TargetAttributes")
	if targetAttrs.IsEmpty() {
		return errors.New("No target attributes found")
	}

	targetAttr := targetAttrs.GetObject(target.UUID)
	if !targetAttr.IsEmpty() {
		targetAttr.Delete(prop)
	}
	return nil
}
