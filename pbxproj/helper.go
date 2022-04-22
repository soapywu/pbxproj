package pbxproj

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/soapywu/pbxproj/pegparser"
)

const COMMENT_KEY_SUFFIX = "_comment"

func isObject(obj interface{}) bool {
	_, ok := obj.(pegparser.Object)
	return ok
}
func toObject(obj interface{}) pegparser.Object {
	return obj.(pegparser.Object)
}

func isArray(obj interface{}) bool {
	_, ok := obj.([]interface{})
	return ok
}

func toArray(obj interface{}) []interface{} {
	return obj.([]interface{})
}

func isString(obj interface{}) bool {
	_, ok := obj.(string)
	return ok
}
func toString(obj interface{}) string {
	return obj.(string)
}

func isInt(obj interface{}) bool {
	switch obj.(type) {
	case int, int8, int16, int32, int64:
		return true
	}
	return false
}
func toIntString(obj interface{}) string {
	switch obj.(type) {
	case int, int8, int16, int32, int64:
		return strconv.FormatInt(reflect.ValueOf(obj).Int(), 10)
	}

	return ""
}

func toCommentKey(key string) string {
	return key + COMMENT_KEY_SUFFIX
}

func fromCommentKey(key string) string {
	return strings.TrimSuffix(key, COMMENT_KEY_SUFFIX)
}

func isCommentKey(key string) bool {
	return strings.HasSuffix(key, COMMENT_KEY_SUFFIX)
}

func nonCommentsFilter(key string, v interface{}) bool {
	return !onlyCommentsFilter(key, v)
}

func onlyCommentsFilter(key string, _ interface{}) bool {
	return isCommentKey(key)
}

func interfaceToStringSlice(val interface{}) []string {
	if val == nil {
		return nil
	}
	switch val := val.(type) {
	case []interface{}:
		result := make([]string, len(val))
		for i, v := range val {
			result[i] = v.(string)
		}
		return result
	case string:
		return []string{val}
	default:
		return nil
	}
}

func stringToInterfaceSlice(val []string) []interface{} {
	if val == nil {
		return nil
	}
	result := make([]interface{}, len(val))
	for i, v := range val {
		result[i] = v
	}
	return result
}

func addToObjectList(obj pegparser.Object, key string, val interface{}) {
	if obj.IsEmpty() {
		return
	}
	list := obj.ForceGet(key)
	if list == nil {
		list = []interface{}{val}
	} else {
		list = append(list.([]interface{}), val)
	}
	obj.Set(key, list)
}

func addToObjectListOnlyNotExist(obj pegparser.Object, key string, val interface{}, equal func(v1, v2 interface{}) bool) {
	if obj.IsEmpty() {
		return
	}
	list := obj.ForceGet(key)
	if list == nil {
		list = []interface{}{val}
	} else {
		for _, v := range list.([]interface{}) {
			if equal(v, val) {
				return
			}
		}
		list = append(list.([]interface{}), val)
	}
	obj.Set(key, list)
}

func removeFromObjectList(obj pegparser.Object, key string, condition func(interface{}) bool, all bool) {
	if obj.IsEmpty() {
		return
	}
	list := obj.ForceGet(key)
	if list == nil {
		return
	}

	for i, v := range list.([]interface{}) {
		if condition(v) {
			list = append(list.([]interface{})[:i], list.([]interface{})[i+1:]...)
			if !all {
				break
			}
		}
	}

	obj.Set(key, list)
}
