package runtime

func mapObject(m *ntlMap) *Value {
	obj := ObjectVal(map[string]*Value{
		"size": NumberVal(0),
		"set": FuncVal(&Function{Name: "set", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) < 2 {
				return this, nil
			}
			key := args[0].ToString()
			m.mu.Lock()
			if _, exists := m.data[key]; !exists {
				m.keyOrder = append(m.keyOrder, key)
			}
			m.data[key] = args[1]
			m.mu.Unlock()
			this.Set("size", NumberVal(float64(len(m.data))))
			return this, nil
		}}),
		"get": FuncVal(&Function{Name: "get", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return Undefined, nil
			}
			m.mu.RLock()
			val, ok := m.data[args[0].ToString()]
			m.mu.RUnlock()
			if !ok {
				return Undefined, nil
			}
			return val, nil
		}}),
		"has": FuncVal(&Function{Name: "has", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return False, nil
			}
			m.mu.RLock()
			_, ok := m.data[args[0].ToString()]
			m.mu.RUnlock()
			return BoolVal(ok), nil
		}}),
		"delete": FuncVal(&Function{Name: "delete", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return False, nil
			}
			key := args[0].ToString()
			m.mu.Lock()
			_, ok := m.data[key]
			delete(m.data, key)
			m.mu.Unlock()
			if ok {
				this.Set("size", NumberVal(float64(len(m.data))))
			}
			return BoolVal(ok), nil
		}}),
		"clear": FuncVal(&Function{Name: "clear", Native: func(args []*Value, this *Value) (*Value, error) {
			m.mu.Lock()
			m.data = make(map[string]*Value)
			m.keyOrder = nil
			m.mu.Unlock()
			this.Set("size", NumberVal(0))
			return Undefined, nil
		}}),
		"keys": FuncVal(&Function{Name: "keys", Native: func(args []*Value, this *Value) (*Value, error) {
			m.mu.RLock()
			result := make([]*Value, len(m.keyOrder))
			for i, k := range m.keyOrder {
				result[i] = StringVal(k)
			}
			m.mu.RUnlock()
			return ArrayVal(result), nil
		}}),
		"values": FuncVal(&Function{Name: "values", Native: func(args []*Value, this *Value) (*Value, error) {
			m.mu.RLock()
			result := make([]*Value, 0, len(m.data))
			for _, k := range m.keyOrder {
				if v, ok := m.data[k]; ok {
					result = append(result, v)
				}
			}
			m.mu.RUnlock()
			return ArrayVal(result), nil
		}}),
		"entries": FuncVal(&Function{Name: "entries", Native: func(args []*Value, this *Value) (*Value, error) {
			m.mu.RLock()
			result := make([]*Value, 0, len(m.data))
			for _, k := range m.keyOrder {
				if v, ok := m.data[k]; ok {
					result = append(result, ArrayVal([]*Value{StringVal(k), v}))
				}
			}
			m.mu.RUnlock()
			return ArrayVal(result), nil
		}}),
		"forEach": FuncVal(&Function{Name: "forEach", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return Undefined, nil
			}
			fn := args[0]
			m.mu.RLock()
			keys := make([]string, len(m.keyOrder))
			copy(keys, m.keyOrder)
			m.mu.RUnlock()
			for _, k := range keys {
				m.mu.RLock()
				v, ok := m.data[k]
				m.mu.RUnlock()
				if ok {
					CallFunction(fn, []*Value{v, StringVal(k), this}, nil)
				}
			}
			return Undefined, nil
		}}),
	})
	return obj
}

func setObject(s *ntlSet) *Value {
	obj := ObjectVal(map[string]*Value{
		"size": NumberVal(float64(len(s.items))),
		"add": FuncVal(&Function{Name: "add", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return this, nil
			}
			key := args[0].ToString()
			s.mu.Lock()
			s.items[key] = args[0]
			s.mu.Unlock()
			this.Set("size", NumberVal(float64(len(s.items))))
			return this, nil
		}}),
		"has": FuncVal(&Function{Name: "has", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return False, nil
			}
			s.mu.RLock()
			_, ok := s.items[args[0].ToString()]
			s.mu.RUnlock()
			return BoolVal(ok), nil
		}}),
		"delete": FuncVal(&Function{Name: "delete", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return False, nil
			}
			key := args[0].ToString()
			s.mu.Lock()
			_, ok := s.items[key]
			delete(s.items, key)
			s.mu.Unlock()
			if ok {
				this.Set("size", NumberVal(float64(len(s.items))))
			}
			return BoolVal(ok), nil
		}}),
		"clear": FuncVal(&Function{Name: "clear", Native: func(args []*Value, this *Value) (*Value, error) {
			s.mu.Lock()
			s.items = make(map[string]*Value)
			s.mu.Unlock()
			this.Set("size", NumberVal(0))
			return Undefined, nil
		}}),
		"forEach": FuncVal(&Function{Name: "forEach", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return Undefined, nil
			}
			fn := args[0]
			s.mu.RLock()
			vals := make([]*Value, 0, len(s.items))
			for _, v := range s.items {
				vals = append(vals, v)
			}
			s.mu.RUnlock()
			for _, v := range vals {
				CallFunction(fn, []*Value{v, v, this}, nil)
			}
			return Undefined, nil
		}}),
		"values": FuncVal(&Function{Name: "values", Native: func(args []*Value, this *Value) (*Value, error) {
			s.mu.RLock()
			vals := make([]*Value, 0, len(s.items))
			for _, v := range s.items {
				vals = append(vals, v)
			}
			s.mu.RUnlock()
			return ArrayVal(vals), nil
		}}),
	})
	return obj
}
