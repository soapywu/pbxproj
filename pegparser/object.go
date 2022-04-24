package pegparser

import (
	"encoding/json"
	"reflect"
)

type IterateActionType = int8

const (
	IterateActionContinue IterateActionType = iota
	IterateActionBreak
)

type ObjectItem = SliceItem

type Object struct {
	*SliceMap
}

type ObjectWithUUID struct {
	Object
	UUID string
}

func NewObjectItem(key string, value interface{}) ObjectItem {
	return SliceItem{key, value}
}

func NewObject() Object {
	return Object{
		SliceMap: NewSliceMap(),
	}
}

func NewObjectWithData(items []ObjectItem) Object {
	o := NewObject()
	for _, item := range items {
		o.Set(item.key, item.data)
	}

	return o
}

func (o Object) toMarshalJSONData() map[string]interface{} {
	dataMap := make(map[string]interface{})
	o.Foreach(func(key string, val interface{}) IterateActionType {
		obj, ok := val.(Object)
		if ok {
			dataMap[key] = obj.toMarshalJSONData()
		} else {
			dataMap[key] = val
		}
		return IterateActionContinue
	})
	return dataMap
}

func (o Object) MarshalJSON() ([]byte, error) {
	return json.Marshal(o.toMarshalJSONData())
}

func (o Object) IsEmpty() bool {
	if o.SliceMap == nil || o.sl == nil {
		return true
	}
	return o.Size() == 0
}

func (o Object) GetObject(key string) Object {
	if value, ok := o.Get(key); ok {
		return value.(Object)
	}
	return NewObject()
}

func (o Object) GetString(key string) string {
	if value, ok := o.Get(key); ok {
		switch v := value.(type) {
		case string:
			return v
		default:
			return ""
		}
	}
	return ""
}

func (o Object) GetInt(key string) int {
	if value, ok := o.Get(key); ok {
		switch value.(type) {
		case int, int8, int16, int32, int64:
			return int(reflect.ValueOf(value).Int())
		}
	}
	return 0
}

type ApplyFunc = func(key string, val interface{}) IterateActionType
type FilterFunc = func(key string, val interface{}) bool

func (o Object) Foreach(apply ApplyFunc) {
	if o.IsEmpty() {
		return
	}
	for _, item := range o.Items() {
		if item.data == nil {
			continue
		}
		action := apply(item.key.(string), item.data)
		if action == IterateActionBreak {
			break
		}
	}
}

func (o Object) ForeachWithFilter(apply ApplyFunc, filter FilterFunc) {
	if o.IsEmpty() {
		return
	}
	for _, item := range o.Items() {
		key := item.key.(string)
		val := item.data
		if val == nil {
			continue
		}
		if filter(key, val) {
			action := apply(key, val)
			if action == IterateActionBreak {
				break
			}
		}
	}
}

func (o Object) Filter(f func(key string, val interface{}) bool) Object {
	newObj := NewObject()
	for _, item := range o.Items() {
		key := item.key.(string)
		val := item.data
		if f(key, val) {
			newObj.Set(key, val)
		}
	}
	return newObj
}

func merge_obj(obj Object, secondObj Object) Object {
	for _, item := range secondObj.Items() {
		key := item.key.(string)
		val := item.data
		if _, ok := obj.Get(key); !ok {
			obj.Set(key, val)
		}
	}

	return obj
}
