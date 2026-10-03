package bytecode

import (
	"bytes"
	"encoding/gob"
	"lunex/internal/ast"
)

func init() {
	gob.Register(&ast.Node{})
	gob.Register(&ast.Param{})
	gob.Register(&ast.MatchCase{})
	gob.Register(&ast.MatchPattern{})
	gob.Register(&ast.MatchProp{})
	gob.Register(&ast.EnumMember{})
	gob.Register(&ast.ImportSpec{})
	gob.Register(&ast.ObjProp{})
	gob.Register(&ast.ClassMember{})
	gob.Register(&ast.SelectCase{})
	gob.Register(&ast.ScopeInfo{})
	gob.Register(&ast.ResolvedAddr{})
	gob.Register(map[string]interface{}{})
	gob.Register([]interface{}{})
	gob.Register([]string{})
}

func encodeAST(tree *ast.Node) ([]byte, error) {
	var b bytes.Buffer
	if err := gob.NewEncoder(&b).Encode(tree); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func decodeAST(data []byte) (*ast.Node, error) {
	var tree *ast.Node
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&tree); err != nil {
		return nil, err
	}
	return tree, nil
}
