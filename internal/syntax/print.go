package syntax

import (
	"fmt"
	"io"
	"strings"
)

// Fprint writes a textual representation of the AST to w.
func Fprint(w io.Writer, node Node) {
	p := &printer{w: w}
	p.print(node)
}

type printer struct {
	w      io.Writer
	indent int
}

func (p *printer) printf(format string, args ...interface{}) {
	fmt.Fprintf(p.w, "%s%s", strings.Repeat("  ", p.indent), fmt.Sprintf(format, args...))
}

func (p *printer) print(node Node) {
	if node == nil {
		return
	}

	switch n := node.(type) {
	case *File:
		p.printf("File %s\n", n.pos)
		p.indent++
		p.printf("Package: %s\n", n.PkgName.Value)
		for _, imp := range n.Imports {
			p.print(imp)
		}
		for _, d := range n.Decls {
			p.print(d)
		}
		p.indent--

	case *ImportDecl:
		p.printf("ImportDecl %s\n", n.pos)
		p.indent++
		if n.Path != nil {
			p.printf("Path: %s\n", n.Path.Value)
		}
		p.indent--

	case *TypeDecl:
		p.printf("TypeDecl %s\n", n.pos)
		p.indent++
		p.printf("Name: %s\n", n.Name.Value)
		if n.Alias {
			p.printf("Alias: true\n")
		}
		p.printf("Type:\n")
		p.indent++
		p.print(n.Type)
		p.indent--
		p.indent--

	case *VarDecl:
		p.printf("VarDecl %s\n", n.pos)
		p.indent++
		p.printf("Name: %s\n", n.Name.Value)
		if n.Type != nil {
			p.printf("Type:\n")
			p.indent++
			p.print(n.Type)
			p.indent--
		}
		if n.Value != nil {
			p.printf("Value:\n")
			p.indent++
			p.print(n.Value)
			p.indent--
		}
		p.indent--

	case *FuncDecl:
		p.printf("FuncDecl %s\n", n.pos)
		p.indent++
		if n.Recv != nil {
			p.printf("Recv: %s %s\n", n.Recv.Name.Value, typeString(n.Recv.Type))
		}
		p.printf("Name: %s\n", n.Name.Value)
		if len(n.Params) > 0 {
			p.printf("Params:\n")
			p.indent++
			for _, f := range n.Params {
				p.printf("%s %s\n", f.Name.Value, typeString(f.Type))
			}
			p.indent--
		}
		if n.Result != nil {
			p.printf("Result: %s\n", typeString(n.Result))
		}
		if n.Body != nil {
			p.printf("Body:\n")
			p.indent++
			p.print(n.Body)
			p.indent--
		}
		p.indent--

	case *BlockStmt:
		p.printf("BlockStmt %s\n", n.pos)
		p.indent++
		for _, s := range n.Stmts {
			p.print(s)
		}
		p.indent--

	case *IfStmt:
		p.printf("IfStmt %s\n", n.pos)
		p.indent++
		p.printf("Cond:\n")
		p.indent++
		p.print(n.Cond)
		p.indent--
		p.printf("Then:\n")
		p.indent++
		p.print(n.Then)
		p.indent--
		if n.Else != nil {
			p.printf("Else:\n")
			p.indent++
			p.print(n.Else)
			p.indent--
		}
		p.indent--

	case *ForStmt:
		p.printf("ForStmt %s\n", n.pos)
		p.indent++
		if n.Cond != nil {
			p.printf("Cond:\n")
			p.indent++
			p.print(n.Cond)
			p.indent--
		}
		p.printf("Body:\n")
		p.indent++
		p.print(n.Body)
		p.indent--
		p.indent--

	case *ReturnStmt:
		p.printf("ReturnStmt %s\n", n.pos)
		if n.Result != nil {
			p.indent++
			p.print(n.Result)
			p.indent--
		}

	case *BranchStmt:
		p.printf("BranchStmt %s %s\n", n.pos, n.Tok)

	case *AssignStmt:
		p.printf("AssignStmt %s %s\n", n.pos, n.Op)
		p.indent++
		p.printf("LHS:\n")
		p.indent++
		for _, e := range n.LHS {
			p.print(e)
		}
		p.indent--
		p.printf("RHS:\n")
		p.indent++
		for _, e := range n.RHS {
			p.print(e)
		}
		p.indent--
		p.indent--

	case *ExprStmt:
		p.printf("ExprStmt %s\n", n.pos)
		p.indent++
		p.print(n.X)
		p.indent--

	case *DeclStmt:
		p.printf("DeclStmt %s\n", n.pos)
		p.indent++
		p.print(n.Decl)
		p.indent--

	case *EmptyStmt:
		p.printf("EmptyStmt %s\n", n.pos)

	case *Name:
		p.printf("Name %s %q\n", n.pos, n.Value)

	case *BasicLit:
		p.printf("BasicLit %s %s %q\n", n.pos, n.Kind, n.Value)

	case *Operation:
		if n.Y == nil {
			p.printf("UnaryOp %s %s\n", n.pos, n.Op)
			p.indent++
			p.print(n.X)
			p.indent--
		} else {
			p.printf("BinaryOp %s %s\n", n.pos, n.Op)
			p.indent++
			p.printf("X:\n")
			p.indent++
			p.print(n.X)
			p.indent--
			p.printf("Y:\n")
			p.indent++
			p.print(n.Y)
			p.indent--
			p.indent--
		}

	case *CallExpr:
		p.printf("CallExpr %s\n", n.pos)
		p.indent++
		p.printf("Fun:\n")
		p.indent++
		p.print(n.Fun)
		p.indent--
		if len(n.Args) > 0 {
			p.printf("Args:\n")
			p.indent++
			for _, a := range n.Args {
				p.print(a)
			}
			p.indent--
		}
		p.indent--

	case *IndexExpr:
		p.printf("IndexExpr %s\n", n.pos)
		p.indent++
		p.printf("X:\n")
		p.indent++
		p.print(n.X)
		p.indent--
		p.printf("Index:\n")
		p.indent++
		p.print(n.Index)
		p.indent--
		p.indent--

	case *SelectorExpr:
		p.printf("SelectorExpr %s\n", n.pos)
		p.indent++
		p.printf("X:\n")
		p.indent++
		p.print(n.X)
		p.indent--
		p.printf("Sel: %s\n", n.Sel.Value)
		p.indent--

	case *ParenExpr:
		p.printf("ParenExpr %s\n", n.pos)
		p.indent++
		p.print(n.X)
		p.indent--

	case *NewExpr:
		p.printf("NewExpr %s\n", n.pos)
		p.indent++
		p.printf("Type: %s\n", typeString(n.Type))
		p.indent--

	case *CompositeLit:
		p.printf("CompositeLit %s\n", n.pos)
		p.indent++
		p.printf("Type: %s\n", typeString(n.Type))
		if len(n.Elems) > 0 {
			p.printf("Elems:\n")
			p.indent++
			for _, e := range n.Elems {
				p.print(e)
			}
			p.indent--
		}
		p.indent--

	case *KeyValueExpr:
		p.printf("KeyValue %s\n", n.pos)
		p.indent++
		p.printf("Key:\n")
		p.indent++
		p.print(n.Key)
		p.indent--
		p.printf("Value:\n")
		p.indent++
		p.print(n.Value)
		p.indent--
		p.indent--

	case *ArrayType:
		p.printf("ArrayType %s\n", n.pos)
		p.indent++
		p.printf("Len:\n")
		p.indent++
		p.print(n.Len)
		p.indent--
		p.printf("Elem: %s\n", typeString(n.Elem))
		p.indent--

	case *PointerType:
		p.printf("PointerType %s\n", n.pos)
		p.indent++
		p.printf("Base: %s\n", typeString(n.Base))
		p.indent--

	case *RefType:
		p.printf("RefType %s\n", n.pos)
		p.indent++
		p.printf("Base: %s\n", typeString(n.Base))
		p.indent--

	case *StructType:
		p.printf("StructType %s\n", n.pos)
		p.indent++
		for _, f := range n.Fields {
			p.printf("Field: %s %s\n", f.Name.Value, typeString(f.Type))
		}
		p.indent--

	case *Field:
		p.printf("Field %s\n", n.pos)
		p.indent++
		if n.Name != nil {
			p.printf("Name: %s\n", n.Name.Value)
		}
		p.printf("Type: %s\n", typeString(n.Type))
		p.indent--

	default:
		p.printf("<%T>\n", node)
	}
}

// typeString returns a string representation of a type expression.
func typeString(e Expr) string {
	if e == nil {
		return "<nil>"
	}
	switch t := e.(type) {
	case *Name:
		return t.Value
	case *PointerType:
		return "*" + typeString(t.Base)
	case *RefType:
		return "ref " + typeString(t.Base)
	case *ArrayType:
		return "[" + exprString(t.Len) + "]" + typeString(t.Elem)
	case *StructType:
		return "struct{...}"
	default:
		return fmt.Sprintf("<%T>", e)
	}
}

// exprString returns a simple string representation of an expression.
func exprString(e Expr) string {
	if e == nil {
		return "<nil>"
	}
	switch x := e.(type) {
	case *Name:
		return x.Value
	case *BasicLit:
		return x.Value
	default:
		return fmt.Sprintf("<%T>", e)
	}
}
