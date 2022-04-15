package pbxproj

import (
	"strings"

	"example.com/peg/pegparser"
)

const COMMENT_KEY_SUFFIX = "_comment"

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

func interfaceToStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	switch v := v.(type) {
	case []interface{}:
		result := make([]string, len(v))
		for i, v := range v {
			result[i] = v.(string)
		}
		return result
	case string:
		return []string{v}
	default:
		return nil
	}
}

func addToObjectList(obj pegparser.Object, key string, val interface{}) {
	if obj == nil {
		return
	}
	list := obj.Get(key)
	if list == nil {
		list = []interface{}{val}
	} else {
		list = append(list.([]interface{}), val)
	}
	obj.Set(key, list)
}

func addToObjectListOnlyNotExist(obj pegparser.Object, key string, val interface{}, equal func(v1, v2 interface{}) bool) {
	if obj == nil {
		return
	}
	list := obj.Get(key)
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
	if obj == nil {
		return
	}
	list := obj.Get(key)
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
