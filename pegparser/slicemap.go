package pegparser

type mapItem struct {
	data interface{}
	idx  int
}

type SliceItem struct {
	key  interface{}
	data interface{}
}

type SliceMap struct {
	mp map[interface{}]*mapItem
	sl []*SliceItem
}

func NewSliceMap() *SliceMap {
	return &SliceMap{
		mp: make(map[interface{}]*mapItem),
		sl: make([]*SliceItem, 0),
	}
}

func (m *SliceMap) ForceGet(key interface{}) interface{} {
	v, found := m.mp[key]
	if found {
		return v.data
	} else {
		return nil
	}
}

func (m *SliceMap) Get(key interface{}) (interface{}, bool) {
	v, found := m.mp[key]
	if found {
		return v.data, true
	} else {
		return nil, false
	}
}

func (m *SliceMap) Set(key, v interface{}) {
	old, found := m.mp[key]
	if found {
		m.mp[key] = &mapItem{
			data: v,
			idx:  old.idx,
		}
		m.sl[old.idx] = &SliceItem{
			data: v,
			key:  key,
		}
	} else {
		m.sl = append(m.sl, &SliceItem{key: key, data: v})
		m.mp[key] = &mapItem{
			data: v,
			idx:  len(m.sl) - 1,
		}
	}
}

func (m *SliceMap) Has(key interface{}) bool {
	_, found := m.mp[key]
	return found
}

func (m *SliceMap) Delete(key interface{}) {
	old, found := m.mp[key]
	if found {
		m.sl = append(m.sl[0:old.idx], m.sl[old.idx+1:]...)
		delete(m.mp, key)
	}
}

func (m *SliceMap) Clear() {
	m.mp = make(map[interface{}]*mapItem)
	m.sl = make([]*SliceItem, 0)
}

func (m *SliceMap) Size() int {
	l := len(m.sl)
	return l
}

func (m *SliceMap) Items() []*SliceItem {
	return m.sl
}

func (m *SliceMap) GetAt(idx int) (interface{}, bool) {
	if idx >= len(m.sl) {
		return nil, false
	}
	val := m.sl[idx].data
	return val, true
}

func (m *SliceMap) DeleteAt(idx int) {
	if idx < len(m.sl) {
		old := m.sl[idx]
		m.sl = append(m.sl[0:idx], m.sl[idx+1:]...)
		delete(m.mp, old.key)
	}
}
