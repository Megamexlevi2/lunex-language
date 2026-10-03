package runtime

import (
	"lunex/internal/errfmt"
	"strings"
	"sync"
	"sync/atomic"
)

type Environment struct {
	mu      sync.RWMutex
	vars    map[string]*Value
	consts  map[string]bool
	parent  *Environment
	escaped int32
	slots   []*Value
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
	e.slots = nil
	atomic.StoreInt32(&e.escaped, 0)
	return e
}

func NewResolvedEnvironment(parent *Environment, slotCount int) *Environment {
	e := NewEnvironment(parent)
	if slotCount > 0 {
		e.slots = make([]*Value, slotCount)
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
	e.slots = nil
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
		if isConst {
			e.consts[name] = true
		}
		return
	}
	e.mu.Lock()
	e.vars[name] = val
	if isConst {
		e.consts[name] = true
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
	for i := 0; i < hops; i++ {
		cur = cur.parent
	}
	if slot < 0 || slot >= len(cur.slots) {
		return Undefined
	}
	if v := cur.slots[slot]; v != nil {
		return v
	}
	return Undefined
}

func (e *Environment) DefineSlot(hops, slot int, name string, val *Value, isConst bool) {
	cur := e
	for i := 0; i < hops; i++ {
		cur = cur.parent
	}
	if slot >= 0 && slot < len(cur.slots) {
		cur.slots[slot] = val
	}
	cur.Define(name, val, isConst)
}

func (e *Environment) SetSlot(hops, slot int, name string, val *Value) error {
	cur := e
	for i := 0; i < hops; i++ {
		cur = cur.parent
	}
	if cur.isConstLocal(name) {
		return errfmt.ConstReassignError(name, "", 0, 0, nil)
	}
	if slot >= 0 && slot < len(cur.slots) {
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
