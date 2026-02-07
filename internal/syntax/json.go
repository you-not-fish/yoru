package syntax

import (
	"encoding/json"
	"io"
)

// FprintJSON writes a JSON representation of the AST to w.
func FprintJSON(w io.Writer, node Node) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(toJSON(node))
}

func toJSON(node Node) interface{} {
	if node == nil {
		return nil
	}

	switch n := node.(type) {
	case *File:
		return map[string]interface{}{
			"type":    "File",
			"pos":     n.pos.String(),
			"package": n.PkgName.Value,
			"imports": mapSlice(n.Imports, func(d *ImportDecl) interface{} { return toJSON(d) }),
			"decls":   mapSliceDecl(n.Decls, toJSON),
		}

	case *ImportDecl:
		m := map[string]interface{}{
			"type": "ImportDecl",
			"pos":  n.pos.String(),
		}
		if n.Path != nil {
			m["path"] = n.Path.Value
		}
		return m

	case *TypeDecl:
		return map[string]interface{}{
			"type":    "TypeDecl",
			"pos":     n.pos.String(),
			"name":    n.Name.Value,
			"alias":   n.Alias,
			"typedef": toJSON(n.Type),
		}

	case *VarDecl:
		m := map[string]interface{}{
			"type": "VarDecl",
			"pos":  n.pos.String(),
			"name": n.Name.Value,
		}
		if n.Type != nil {
			m["vartype"] = toJSON(n.Type)
		}
		if n.Value != nil {
			m["value"] = toJSON(n.Value)
		}
		return m

	case *FuncDecl:
		m := map[string]interface{}{
			"type": "FuncDecl",
			"pos":  n.pos.String(),
			"name": n.Name.Value,
		}
		if n.Recv != nil {
			m["recv"] = toJSON(n.Recv)
		}
		m["params"] = mapSlice(n.Params, func(f *Field) interface{} { return toJSON(f) })
		if n.Result != nil {
			m["result"] = toJSON(n.Result)
		}
		if n.Body != nil {
			m["body"] = toJSON(n.Body)
		}
		return m

	case *Field:
		m := map[string]interface{}{
			"type": "Field",
			"pos":  n.pos.String(),
		}
		if n.Name != nil {
			m["name"] = n.Name.Value
		}
		m["fieldtype"] = toJSON(n.Type)
		return m

	case *BlockStmt:
		return map[string]interface{}{
			"type":  "BlockStmt",
			"pos":   n.pos.String(),
			"stmts": mapSliceStmt(n.Stmts, toJSON),
		}

	case *IfStmt:
		m := map[string]interface{}{
			"type": "IfStmt",
			"pos":  n.pos.String(),
			"cond": toJSON(n.Cond),
			"then": toJSON(n.Then),
		}
		if n.Else != nil {
			m["else"] = toJSON(n.Else)
		}
		return m

	case *ForStmt:
		m := map[string]interface{}{
			"type": "ForStmt",
			"pos":  n.pos.String(),
			"body": toJSON(n.Body),
		}
		if n.Cond != nil {
			m["cond"] = toJSON(n.Cond)
		}
		return m

	case *ReturnStmt:
		m := map[string]interface{}{
			"type": "ReturnStmt",
			"pos":  n.pos.String(),
		}
		if n.Result != nil {
			m["result"] = toJSON(n.Result)
		}
		return m

	case *BranchStmt:
		return map[string]interface{}{
			"type":  "BranchStmt",
			"pos":   n.pos.String(),
			"token": n.Tok.String(),
		}

	case *AssignStmt:
		return map[string]interface{}{
			"type": "AssignStmt",
			"pos":  n.pos.String(),
			"op":   n.Op.String(),
			"lhs":  mapSliceExpr(n.LHS, toJSON),
			"rhs":  mapSliceExpr(n.RHS, toJSON),
		}

	case *ExprStmt:
		return map[string]interface{}{
			"type": "ExprStmt",
			"pos":  n.pos.String(),
			"x":    toJSON(n.X),
		}

	case *DeclStmt:
		return map[string]interface{}{
			"type": "DeclStmt",
			"pos":  n.pos.String(),
			"decl": toJSON(n.Decl),
		}

	case *EmptyStmt:
		return map[string]interface{}{
			"type": "EmptyStmt",
			"pos":  n.pos.String(),
		}

	case *Name:
		return map[string]interface{}{
			"type":  "Name",
			"pos":   n.pos.String(),
			"value": n.Value,
		}

	case *BasicLit:
		return map[string]interface{}{
			"type":  "BasicLit",
			"pos":   n.pos.String(),
			"kind":  n.Kind.String(),
			"value": n.Value,
		}

	case *Operation:
		m := map[string]interface{}{
			"type": "Operation",
			"pos":  n.pos.String(),
			"op":   n.Op.String(),
			"x":    toJSON(n.X),
		}
		if n.Y != nil {
			m["y"] = toJSON(n.Y)
		}
		return m

	case *CallExpr:
		return map[string]interface{}{
			"type": "CallExpr",
			"pos":  n.pos.String(),
			"fun":  toJSON(n.Fun),
			"args": mapSliceExpr(n.Args, toJSON),
		}

	case *IndexExpr:
		return map[string]interface{}{
			"type":  "IndexExpr",
			"pos":   n.pos.String(),
			"x":     toJSON(n.X),
			"index": toJSON(n.Index),
		}

	case *SelectorExpr:
		return map[string]interface{}{
			"type": "SelectorExpr",
			"pos":  n.pos.String(),
			"x":    toJSON(n.X),
			"sel":  n.Sel.Value,
		}

	case *ParenExpr:
		return map[string]interface{}{
			"type": "ParenExpr",
			"pos":  n.pos.String(),
			"x":    toJSON(n.X),
		}

	case *NewExpr:
		return map[string]interface{}{
			"type":    "NewExpr",
			"pos":     n.pos.String(),
			"newtype": toJSON(n.Type),
		}

	case *CompositeLit:
		return map[string]interface{}{
			"type":    "CompositeLit",
			"pos":     n.pos.String(),
			"littype": toJSON(n.Type),
			"elems":   mapSliceExpr(n.Elems, toJSON),
		}

	case *KeyValueExpr:
		return map[string]interface{}{
			"type":  "KeyValueExpr",
			"pos":   n.pos.String(),
			"key":   toJSON(n.Key),
			"value": toJSON(n.Value),
		}

	case *ArrayType:
		return map[string]interface{}{
			"type": "ArrayType",
			"pos":  n.pos.String(),
			"len":  toJSON(n.Len),
			"elem": toJSON(n.Elem),
		}

	case *PointerType:
		return map[string]interface{}{
			"type": "PointerType",
			"pos":  n.pos.String(),
			"base": toJSON(n.Base),
		}

	case *RefType:
		return map[string]interface{}{
			"type": "RefType",
			"pos":  n.pos.String(),
			"base": toJSON(n.Base),
		}

	case *StructType:
		return map[string]interface{}{
			"type":   "StructType",
			"pos":    n.pos.String(),
			"fields": mapSlice(n.Fields, func(f *Field) interface{} { return toJSON(f) }),
		}

	default:
		return map[string]interface{}{
			"type": "Unknown",
		}
	}
}

// Helper functions to map slices

func mapSlice[T any](s []T, f func(T) interface{}) []interface{} {
	result := make([]interface{}, len(s))
	for i, v := range s {
		result[i] = f(v)
	}
	return result
}

func mapSliceDecl(s []Decl, f func(Node) interface{}) []interface{} {
	result := make([]interface{}, len(s))
	for i, v := range s {
		result[i] = f(v)
	}
	return result
}

func mapSliceStmt(s []Stmt, f func(Node) interface{}) []interface{} {
	result := make([]interface{}, len(s))
	for i, v := range s {
		result[i] = f(v)
	}
	return result
}

func mapSliceExpr(s []Expr, f func(Node) interface{}) []interface{} {
	result := make([]interface{}, len(s))
	for i, v := range s {
		result[i] = f(v)
	}
	return result
}
