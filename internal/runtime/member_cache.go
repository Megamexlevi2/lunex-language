package runtime

import "lunex/internal/ast"

type memberAccessCache struct {
	key           string
	objectShape   *ObjectShape
	instanceShape *InstanceShape
	slot          int
}

func (interp *Interpreter) cachedMember(node *ast.Node, object *Value, key string) (*Value, bool) {
	if interp == nil || node == nil || object == nil {
		return nil, false
	}
	cached, ok := interp.memberCache.Load(node)
	if !ok {
		return nil, false
	}
	entry := cached.(memberAccessCache)
	if entry.key != key {
		return nil, false
	}
	switch object.Tag {
	case TypeObject:
		if object.ObjShape != nil && object.ObjShape == entry.objectShape && entry.slot >= 0 && entry.slot < len(object.ObjValues) {
			return object.ObjValues[entry.slot], true
		}
	case TypeInstance:
		if object.InstVal == nil || object.InstVal.Shape == nil || object.InstVal.Shape != entry.instanceShape {
			return nil, false
		}
		if entry.slot < 0 || entry.slot >= len(object.InstVal.Values) {
			return nil, false
		}
		if value := object.InstVal.Values[entry.slot]; value != nil {
			return value, true
		}
		if value, exists := object.InstVal.Fields[key]; exists {
			if value == nil {
				return Undefined, true
			}
			return value, true
		}
		return Undefined, true
	}
	return nil, false
}

func (interp *Interpreter) rememberMember(node *ast.Node, object *Value, key string) {
	if interp == nil || node == nil || object == nil {
		return
	}
	entry := memberAccessCache{key: key, slot: -1}
	switch object.Tag {
	case TypeObject:
		if object.ObjShape == nil {
			return
		}
		slot, exists := object.ObjShape.Index[key]
		if !exists || slot < 0 || slot >= len(object.ObjValues) {
			return
		}
		entry.objectShape = object.ObjShape
		entry.slot = slot
	case TypeInstance:
		if object.InstVal == nil || object.InstVal.Shape == nil {
			return
		}
		slot, exists := object.InstVal.Shape.Index[key]
		if !exists || slot < 0 || slot >= len(object.InstVal.Values) {
			return
		}
		entry.instanceShape = object.InstVal.Shape
		entry.slot = slot
	default:
		return
	}
	interp.memberCache.Store(node, entry)
}
