package syntax

// Visitor is called for each node during Walk.
// If it returns false, the children of the node are not visited.
type Visitor func(node Node) bool

// Walk traverses an AST in depth-first order.
// If visitor returns false, children are not visited.
func Walk(node Node, v Visitor) {
	if node == nil || !v(node) {
		return
	}

	switch n := node.(type) {
	case *File:
		Walk(n.PkgName, v)
		for _, imp := range n.Imports {
			Walk(imp, v)
		}
		for _, d := range n.Decls {
			Walk(d, v)
		}

	case *ImportDecl:
		if n.Path != nil {
			Walk(n.Path, v)
		}

	case *TypeDecl:
		Walk(n.Name, v)
		Walk(n.Type, v)

	case *VarDecl:
		Walk(n.Name, v)
		if n.Type != nil {
			Walk(n.Type, v)
		}
		if n.Value != nil {
			Walk(n.Value, v)
		}

	case *FuncDecl:
		if n.Recv != nil {
			Walk(n.Recv, v)
		}
		Walk(n.Name, v)
		for _, p := range n.Params {
			Walk(p, v)
		}
		if n.Result != nil {
			Walk(n.Result, v)
		}
		if n.Body != nil {
			Walk(n.Body, v)
		}

	case *Field:
		if n.Name != nil {
			Walk(n.Name, v)
		}
		Walk(n.Type, v)

	case *BlockStmt:
		for _, s := range n.Stmts {
			Walk(s, v)
		}

	case *IfStmt:
		Walk(n.Cond, v)
		Walk(n.Then, v)
		if n.Else != nil {
			Walk(n.Else, v)
		}

	case *ForStmt:
		if n.Cond != nil {
			Walk(n.Cond, v)
		}
		Walk(n.Body, v)

	case *ReturnStmt:
		if n.Result != nil {
			Walk(n.Result, v)
		}

	case *AssignStmt:
		for _, e := range n.LHS {
			Walk(e, v)
		}
		for _, e := range n.RHS {
			Walk(e, v)
		}

	case *ExprStmt:
		Walk(n.X, v)

	case *DeclStmt:
		Walk(n.Decl, v)

	case *Operation:
		Walk(n.X, v)
		if n.Y != nil {
			Walk(n.Y, v)
		}

	case *CallExpr:
		Walk(n.Fun, v)
		for _, a := range n.Args {
			Walk(a, v)
		}

	case *IndexExpr:
		Walk(n.X, v)
		Walk(n.Index, v)

	case *SelectorExpr:
		Walk(n.X, v)
		Walk(n.Sel, v)

	case *ParenExpr:
		Walk(n.X, v)

	case *NewExpr:
		Walk(n.Type, v)

	case *CompositeLit:
		if n.Type != nil {
			Walk(n.Type, v)
		}
		for _, e := range n.Elems {
			Walk(e, v)
		}

	case *KeyValueExpr:
		Walk(n.Key, v)
		Walk(n.Value, v)

	case *ArrayType:
		Walk(n.Len, v)
		Walk(n.Elem, v)

	case *PointerType:
		Walk(n.Base, v)

	case *RefType:
		Walk(n.Base, v)

	case *StructType:
		for _, f := range n.Fields {
			Walk(f, v)
		}

	// Leaf nodes: Name, BasicLit, EmptyStmt, BranchStmt
	// No children to visit
	}
}

// Inspect traverses an AST and calls f for each node.
// Convenience wrapper around Walk.
func Inspect(node Node, f func(Node) bool) {
	Walk(node, Visitor(f))
}
