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
	"fmt"
	"os"
	"reflect"
	"strings"

	"example.com/peg/pegparser"
)

const (
	INDENT = "\t"
)

type StringWriter interface {
	WriteString(string) (int, error)
	String() string
}

type PbxWriterOption func(w *PbxWriter)

func WithOmitEmpty() PbxWriterOption {
	return func(w *PbxWriter) {
		w.omitEmptyValues = true
	}
}

func WithStringWriter(writer StringWriter) PbxWriterOption {
	return func(w *PbxWriter) {
		w.stringWriter = writer
	}
}

type PbxWriter struct {
	stringWriter    StringWriter
	omitEmptyValues bool
	contents        pegparser.Object
	sync            bool
	indentLevel     int
}

func NewPbxWriter(project *PbxProject, options ...PbxWriterOption) *PbxWriter {
	w := &PbxWriter{
		contents:     project.Contents(),
		stringWriter: &strings.Builder{},
		indentLevel:  0,
		sync:         false,
	}
	for _, option := range options {
		option(w)
	}
	return w
}

func indent(x int) string {
	if x <= 0 {
		return ""
	} else {
		return INDENT + indent(x-1)
	}
}

func getComment(key string, parent pegparser.Object) string {
	return parent.GetString(toCommentKey(key))
}

// func (w *PbxWriter) writeString(str string) {
// 	_, _ = w.stringWriter.WriteString(str)
// }
func (w *PbxWriter) writeFormatString(format string, str ...string) {
	_, _ = w.stringWriter.WriteString(fmt.Sprintf(format, stringToInterfaceSlice(str)...))
}

func (w PbxWriter) write(format string, str ...string) {
	fmtStr := fmt.Sprintf(format, stringToInterfaceSlice(str)...)
	w.writeFormatString("%s%s", indent(w.indentLevel), fmtStr)
}

func (w PbxWriter) writeNoIndent(format string, str ...string) {
	fmtStr := fmt.Sprintf(format, stringToInterfaceSlice(str)...)
	w.writeFormatString("%s%s", indent(0), fmtStr)
}

func (w *PbxWriter) Write(filePath string) error {
	w.writeHeadComment()
	w.writeProject()
	return os.WriteFile(filePath, []byte(w.stringWriter.String()), 0644)
}

func (w *PbxWriter) writeHeadComment() {
	comment := w.contents.GetString("headComment")
	if comment != "" {
		w.writeNoIndent("// %s\n", comment)
	}
}

func (w *PbxWriter) writeProject() {
	proj := w.contents.GetObject("project")

	w.write("{\n")
	w.indentLevel++

	proj.ForeachWithFilter(func(key string, val interface{}) pegparser.IterateActionType {
		cmt := getComment(key, proj)
		if isArray(val) {
			w.writeArray(toArray(val), key)
		} else if isObject(val) {
			w.write("%s = {\n", key)
			w.indentLevel++
			if key == "objects" {
				w.writeObjectsSections(toObject(val))
			} else {
				w.writeObject(toObject(val))
			}
			w.indentLevel--
			w.write("};\n")
		} else if isString(val) {
			str := toString(val)
			if w.omitEmptyValues && str == "" {
				return pegparser.IterateActionContinue
			} else {
				if cmt != "" {
					w.write("%s = %s /* %s */;\n", key, str, cmt)
				} else {
					w.write("%s = %s;\n", key, str)
				}
			}
		} else if isInt(val) {
			if cmt != "" {
				w.write("%s = %s /* %s */;\n", key, toIntString(val), cmt)
			} else {
				w.write("%s = %s;\n", key, toIntString(val))
			}
		} else {
			fmt.Printf("writeProject unknown %s: %v \n", key, val)
			fmt.Println(reflect.TypeOf(val))
		}
		return pegparser.IterateActionContinue
	}, nonCommentsFilter)

	w.indentLevel--

	w.write("}\n")
}

func (w PbxWriter) writeObject(obj pegparser.Object) {
	obj.ForeachWithFilter(func(key string, val interface{}) pegparser.IterateActionType {
		cmt := getComment(key, obj)
		if isArray(val) {
			w.writeArray(toArray(val), key)
		} else if isObject(val) {
			w.write("%s = {\n", key)
			w.indentLevel++
			w.writeObject(toObject(val))
			w.indentLevel--
			w.write("};\n")
		} else if isString(val) {
			str := toString(val)
			if w.omitEmptyValues && str == "" {
				return pegparser.IterateActionContinue
			} else if cmt != "" {
				w.write("%s = %s /* %s */;\n", key, str, cmt)
			} else {
				w.write("%s = %s;\n", key, str)
			}
		} else if isInt(val) {
			if cmt != "" {
				w.write("%s = %s /* %s */;\n", key, toIntString(val), cmt)
			} else {
				w.write("%s = %s;\n", key, toIntString(val))
			}
		} else {
			fmt.Printf("writeObject unknown %s: %v\n", key, val)
			fmt.Println(reflect.TypeOf(val))
		}
		return pegparser.IterateActionContinue
	}, nonCommentsFilter)

}

func (w PbxWriter) writeObjectsSections(obj pegparser.Object) {
	obj.Foreach(func(key string, val interface{}) pegparser.IterateActionType {
		if isObject(val) {
			value := val.(pegparser.Object)
			if value.IsEmpty() {
				return pegparser.IterateActionContinue
			}
			w.writeNoIndent("\n")
			w.writeSectionComment(key, true)
			w.writeSection(val.(pegparser.Object))
			w.writeSectionComment(key, false)
		}
		return pegparser.IterateActionContinue
	})
}

func (w PbxWriter) writeArray(arr []interface{}, name string) {
	// if w.omitEmptyValues && len(arr) == 0 {
	// 	return
	// }

	w.write("%s = (\n", name)
	w.indentLevel++

	for _, obj := range arr {
		if isObject(obj) {
			val := obj.(pegparser.Object)
			value := val.GetString("value")
			comment := val.GetString("comment")
			if value != "" && comment != "" {
				w.write("%s /* %s */,\n", value, comment)
			} else {
				w.write("{\n")
				w.indentLevel++
				w.writeObject(val)
				w.indentLevel--
				w.write("},\n")
			}
		} else if isString(obj) {
			w.write("%s,\n", obj.(string))
		} else if isInt(obj) {
			w.write("%s,\n", toIntString(obj))
		} else {
			fmt.Printf("writeArray unsupport %v\n", obj)
			fmt.Println(reflect.TypeOf(obj))
		}
	}
	w.indentLevel--
	w.write(");\n")
}

func (w PbxWriter) writeSectionComment(name string, begin bool) {
	if begin {
		w.writeNoIndent("/* Begin %s section */\n", name)
	} else { // end
		w.writeNoIndent("/* End %s section */\n", name)
	}
}

func (w PbxWriter) writeSection(section pegparser.Object) {
	section.ForeachWithFilter(func(key string, val interface{}) pegparser.IterateActionType {
		cmt := getComment(key, section)
		if !isObject(val) {
			return pegparser.IterateActionContinue
		}
		obj := val.(pegparser.Object)
		isa := obj.GetString("isa")
		if isa == "PBXBuildFile" || isa == "PBXFileReference" {
			w.writeInlineObject(key, cmt, obj)
		} else {
			if cmt != "" {
				w.write("%s /* %s */ = {\n", key, cmt)
			} else {
				w.write("%s = {\n", key)
			}

			w.indentLevel++
			w.writeObject(obj)
			w.indentLevel--
			w.write("};\n")
		}
		return pegparser.IterateActionContinue
	}, nonCommentsFilter)
}

func (w PbxWriter) writeInlineObjectHelp(buffer *[]string, name string, desc string, ref pegparser.Object) {
	output := *buffer
	if desc != "" {
		output = append(output, fmt.Sprintf("%s /* %s */ = {", name, desc))
	} else {
		output = append(output, fmt.Sprintf("%s = {", name))
	}

	ref.ForeachWithFilter(func(key string, val interface{}) pegparser.IterateActionType {
		cmt := getComment(key, ref)
		if isArray(val) {
			output = append(output, fmt.Sprintf("%s = (", key))
			output = append(output, strings.Join(interfaceToStringSlice(val), ","))
			output = append(output, "),")
		} else if isObject(val) {
			w.writeInlineObjectHelp(&output, key, cmt, val.(pegparser.Object))
		} else if isString(val) {
			value := val.(string)
			if value == "" && w.omitEmptyValues {
				return pegparser.IterateActionContinue
			}
			if cmt != "" {
				output = append(output, fmt.Sprintf("%s = %s /* %s */; ", key, value, cmt))
			} else {
				output = append(output, fmt.Sprintf("%s = %s; ", key, value))
			}
		} else if isInt(val) {
			if cmt != "" {
				output = append(output, fmt.Sprintf("%s = %s /* %s */; ", key, toIntString(val), cmt))
			} else {
				output = append(output, fmt.Sprintf("%s = %s; ", key, toIntString(val)))
			}
		} else {
			fmt.Printf("unhandled inline object type %s->%+v\n", key, val)
			fmt.Println(reflect.TypeOf(val))
		}
		return pegparser.IterateActionContinue
	}, nonCommentsFilter)

	output = append(output, "};")
	*buffer = output
}

func (w PbxWriter) writeInlineObject(name string, desc string, ref pegparser.Object) {
	output := []string{}
	w.writeInlineObjectHelp(&output, name, desc, ref)
	w.write("%s\n", strings.TrimSpace(strings.Join(output, "")))
}
