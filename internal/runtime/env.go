package runtime

import (
	"lunex/internal/errfmt"
	"strings"
	"sync"
	"sync/atomic"
)

type Environment struct {
	mu         sync.RWMutex
	vars       map[string]*Value
	consts     map[string]bool
	parent     *Environment
	escaped    int32
	slots      []*Value
	slotNames  []string
	slotIndex  map[string]int
	slotConsts []bool
}

type environmentLayoutKey struct {
	first *string
	count int
}

type environmentLayout struct {
	names []string
	index map[string]int
}

var environmentLayouts = struct {
	sync.RWMutex
	items map[environmentLayoutKey]*environmentLayout
}{items: make(map[environmentLayoutKey]*environmentLayout)}

func environmentLayoutFor(names []string, count int) *environmentLayout {
	if count > len(names) {
		count = len(names)
	}
	if count <= 0 {
		return &environmentLayout{index: map[string]int{}}
	}
	key := environmentLayoutKey{first: &names[0], count: count}
	environmentLayouts.RLock()
	layout := environmentLayouts.items[key]
	environmentLayouts.RUnlock()
	if layout != nil {
		return layout
	}
	layout = &environmentLayout{names: names[:count], index: make(map[string]int, count)}
	for slot, name := range layout.names {
		if name != "" {
			layout.index[name] = slot
		}
	}
	environmentLayouts.Lock()
	if existing := environmentLayouts.items[key]; existing != nil {
		layout = existing
	} else {
		if len(environmentLayouts.items) >= 4096 {
			environmentLayouts.items = make(map[environmentLayoutKey]*environmentLayout)
		}
		environmentLayouts.items[key] = layout
	}
	environmentLayouts.Unlock()
	return layout
}

var envPool = sync.Pool{
	New: func() any {
		return &Environment{
			vars:   make(map[string]*Value, 8),
			consts: make(map[string]bool, 4),
		}
	},
}

func NewEnvironment(parent *Environment) *Environment {
	e := envPool.Get().(*Environment)

	for k := range e.vars {
		delete(e.vars, k)
	}
	for k := range e.consts {
		delete(e.consts, k)
	}
	e.parent = parent
	e.slots = e.slots[:0]
	e.slotNames = nil
	e.slotIndex = nil
	e.slotConsts = e.slotConsts[:0]
	atomic.StoreInt32(&e.escaped, 0)
	return e
}

func NewResolvedEnvironment(parent *Environment, slotCount int, names ...[]string) *Environment {
	e := NewEnvironment(parent)
	if slotCount > 0 {
		if cap(e.slots) < slotCount {
			e.slots = make([]*Value, slotCount)
		} else {
			e.slots = e.slots[:slotCount]
			clear(e.slots)
		}
		if cap(e.slotConsts) < slotCount {
			e.slotConsts = make([]bool, slotCount)
		} else {
			e.slotConsts = e.slotConsts[:slotCount]
			clear(e.slotConsts)
		}
	}
	if len(names) > 0 && slotCount > 0 {
		layout := environmentLayoutFor(names[0], slotCount)
		e.slotNames = layout.names
		e.slotIndex = layout.index
	}
	return e
}

func ReleaseEnvironment(e *Environment) {
	if e == nil {
		return
	}
	if atomic.LoadInt32(&e.escaped) != 0 {
		return
	}
	for k := range e.vars {
		delete(e.vars, k)
	}
	for k := range e.consts {
		delete(e.consts, k)
	}
	e.parent = nil
	clear(e.slots)
	e.slots = e.slots[:0]
	e.slotNames = nil
	e.slotIndex = nil
	clear(e.slotConsts)
	e.slotConsts = e.slotConsts[:0]
	envPool.Put(e)
}

func MarkEscaped(e *Environment) {
	for cur := e; cur != nil; {
		if !atomic.CompareAndSwapInt32(&cur.escaped, 0, 1) {
			break
		}
		cur = cur.parent
	}
}

func (e *Environment) isEscaped() bool {
	return atomic.LoadInt32(&e.escaped) != 0
}

func (e *Environment) Define(name string, val *Value, isConst bool) {
	if !e.isEscaped() {
		e.vars[name] = val
		if slot, ok := e.slotIndex[name]; ok && slot >= 0 && slot < len(e.slots) {
			e.slots[slot] = val
		}
		if isConst {
			e.consts[name] = true
			if slot, ok := e.slotIndex[name]; ok && slot >= 0 && slot < len(e.slotConsts) {
				e.slotConsts[slot] = true
			}
		}
		return
	}
	e.mu.Lock()
	e.vars[name] = val
	if slot, ok := e.slotIndex[name]; ok && slot >= 0 && slot < len(e.slots) {
		e.slots[slot] = val
	}
	if isConst {
		e.consts[name] = true
		if slot, ok := e.slotIndex[name]; ok && slot >= 0 && slot < len(e.slotConsts) {
			e.slotConsts[slot] = true
		}
	}
	e.mu.Unlock()
}

func (e *Environment) SetLocal(name string, val *Value) {
	if !e.isEscaped() {
		e.vars[name] = val
		return
	}
	e.mu.Lock()
	e.vars[name] = val
	e.mu.Unlock()
}

func (e *Environment) GetLocal(name string) (*Value, bool) {
	if !e.isEscaped() {
		v, ok := e.vars[name]
		return v, ok
	}
	e.mu.RLock()
	v, ok := e.vars[name]
	e.mu.RUnlock()
	return v, ok
}

func (e *Environment) Set(name string, val *Value) error {
	if !e.isEscaped() {
		if _, ok := e.vars[name]; ok {
			if e.consts[name] {
				return errfmt.ConstReassignError(name, "", 0, 0, nil)
			}
			e.vars[name] = val
			return nil
		}
	} else {
		e.mu.Lock()
		if _, ok := e.vars[name]; ok {
			if e.consts[name] {
				e.mu.Unlock()
				return errfmt.ConstReassignError(name, "", 0, 0, nil)
			}
			e.vars[name] = val
			e.mu.Unlock()
			return nil
		}
		e.mu.Unlock()
	}

	if e.parent != nil {
		env := e.parent.find(name)
		if env != nil {
			if !env.isEscaped() {
				if env.consts[name] {
					return errfmt.ConstReassignError(name, "", 0, 0, nil)
				}
				env.vars[name] = val
				return nil
			}
			env.mu.Lock()
			if env.consts[name] {
				env.mu.Unlock()
				return errfmt.ConstReassignError(name, "", 0, 0, nil)
			}
			env.vars[name] = val
			env.mu.Unlock()
			return nil
		}
	}
	return errfmt.ReferenceError(name, "", 0, 0, nil)
}

func (e *Environment) Get(name string) (*Value, bool) {
	if !e.isEscaped() {
		if v, ok := e.vars[name]; ok {
			return v, true
		}
	} else {
		e.mu.RLock()
		if v, ok := e.vars[name]; ok {
			e.mu.RUnlock()
			return v, true
		}
		e.mu.RUnlock()
	}

	if e.parent != nil {
		env := e.parent.find(name)
		if env != nil {
			if !env.isEscaped() {
				return env.vars[name], true
			}
			env.mu.RLock()
			v := env.vars[name]
			env.mu.RUnlock()
			return v, true
		}
	}
	return Undefined, false
}

func (e *Environment) find(name string) *Environment {
	cur := e
	for cur != nil {
		if !cur.isEscaped() {
			if _, ok := cur.vars[name]; ok {
				return cur
			}
		} else {
			cur.mu.RLock()
			_, ok := cur.vars[name]
			cur.mu.RUnlock()
			if ok {
				return cur
			}
		}
		cur = cur.parent
	}
	return nil
}

func (e *Environment) Has(name string) bool {
	return e.find(name) != nil
}

func (e *Environment) Parent() *Environment {
	return e.parent
}

func (e *Environment) AllNames() []string {
	seen := make(map[string]bool)
	cur := e
	for cur != nil {
		if !cur.isEscaped() {
			for k := range cur.vars {
				if !strings.HasPrefix(k, "__") {
					seen[k] = true
				}
			}
		} else {
			cur.mu.RLock()
			for k := range cur.vars {
				if !strings.HasPrefix(k, "__") {
					seen[k] = true
				}
			}
			cur.mu.RUnlock()
		}
		cur = cur.parent
	}
	names := make([]string, 0, len(seen))
	for k := range seen {
		names = append(names, k)
	}
	return names
}

func (e *Environment) GetSlotAddr(hops, slot int) *Value {
	cur := e
	for i := 0; i < hops && cur != nil; i++ {
		cur = cur.parent
	}
	if cur == nil || slot < 0 || slot >= len(cur.slots) {
		return Undefined
	}
	if v := cur.slots[slot]; v != nil {
		return v
	}
	return Undefined
}

func (e *Environment) DefineSlot(hops, slot int, name string, val *Value, isConst bool) {
	cur := e
	for i := 0; i < hops && cur != nil; i++ {
		cur = cur.parent
	}
	if cur == nil {
		return
	}
	if slot >= 0 && slot < len(cur.slots) && (slot >= len(cur.slotNames) || cur.slotNames[slot] == name) {
		cur.slots[slot] = val
		if slot < len(cur.slotConsts) && isConst {
			cur.slotConsts[slot] = true
		}
	}
	cur.Define(name, val, isConst)
}

func (e *Environment) SetSlot(hops, slot int, name string, val *Value) error {
	cur := e
	for i := 0; i < hops && cur != nil; i++ {
		cur = cur.parent
	}
	if cur == nil {
		return errfmt.ReferenceError(name, "", 0, 0, nil)
	}
	validSlot := slot >= 0 && slot < len(cur.slots) && (slot >= len(cur.slotNames) || cur.slotNames[slot] == name)
	if validSlot && slot < len(cur.slotConsts) {
		if cur.slotConsts[slot] {
			return errfmt.ConstReassignError(name, "", 0, 0, nil)
		}
	} else if cur.isConstLocal(name) {
		return errfmt.ConstReassignError(name, "", 0, 0, nil)
	}
	if validSlot {
		cur.slots[slot] = val
	}
	cur.SetLocal(name, val)
	return nil
}

func (e *Environment) isConstLocal(name string) bool {
	if !e.isEscaped() {
		return e.consts[name]
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.consts[name]
}

func (e *Environment) Snapshot() map[string]*Value {
	if !e.isEscaped() {
		out := make(map[string]*Value, len(e.vars))
		for k, v := range e.vars {
			out[k] = v
		}
		return out
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make(map[string]*Value, len(e.vars))
	for k, v := range e.vars {
		out[k] = v
	}
	return out
}
