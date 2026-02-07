package syntax

import "io"

// Maximum number of errors before aborting parse.
const maxErrors = 10

// SyntaxError represents a syntax error.
type SyntaxError struct {
	Pos Pos
	Msg string
}

func (e *SyntaxError) Error() string {
	return e.Pos.String() + ": " + e.Msg
}

// Parser performs syntax analysis on Yoru source code.
type Parser struct {
	scanner *Scanner

	// Current token info (cached from scanner)
	tok Token
	lit string
	pos Pos

	// Error handling
	errh   func(pos Pos, msg string)
	errcnt int
	first  error // first error encountered
	abort  bool  // set to true when error limit reached

	// Context tracking
	fnest int // function nesting depth (0 = top-level)
}

// NewParser creates a new Parser for the given source.
func NewParser(filename string, src io.Reader, errh func(pos Pos, msg string)) *Parser {
	scanErrh := func(line, col uint32, msg string) {
		if errh != nil {
			errh(NewPos(filename, line, col), msg)
		}
	}

	p := &Parser{
		scanner: NewScanner(filename, src, scanErrh),
		errh:    errh,
	}
	p.next() // prime the parser with first token
	return p
}

// SetASIEnabled passes the ASI setting to the underlying scanner.
func (p *Parser) SetASIEnabled(enabled bool) {
	p.scanner.SetASIEnabled(enabled)
}

// ----------------------------------------------------------------------------
// Token navigation

// next advances to the next token.
func (p *Parser) next() {
	p.scanner.Next()
	p.tok = p.scanner.Token()
	p.lit = p.scanner.Literal()
	p.pos = p.scanner.Pos()
}

// got reports whether the current token is tok.
// If so, it consumes the token and returns true.
func (p *Parser) got(tok Token) bool {
	if p.tok == tok {
		p.next()
		return true
	}
	return false
}

// want consumes the current token if it matches tok.
// Otherwise, reports an error.
func (p *Parser) want(tok Token) {
	if !p.got(tok) {
		p.syntaxError("expected " + tok.String())
		p.advance()
	}
}

// expect is like want but returns the position of the expected token.
func (p *Parser) expect(tok Token) Pos {
	pos := p.pos
	p.want(tok)
	return pos
}

// ----------------------------------------------------------------------------
// Error handling

// syntaxError reports a syntax error at the current position.
func (p *Parser) syntaxError(msg string) {
	p.syntaxErrorAt(p.pos, msg)
}

// syntaxErrorAt reports a syntax error at a specific position.
func (p *Parser) syntaxErrorAt(pos Pos, msg string) {
	if p.abort {
		return
	}
	if p.errcnt == 0 {
		p.first = &SyntaxError{Pos: pos, Msg: msg}
	}
	p.errcnt++

	if p.errh != nil {
		p.errh(pos, msg)
	}

	p.errorLimitCheck(pos)
}

// errorLimitCheck aborts parsing if too many errors have occurred.
func (p *Parser) errorLimitCheck(pos Pos) {
	if p.errcnt >= maxErrors {
		p.abort = true
		if p.errh != nil {
			p.errh(pos, "too many errors; aborting parse")
		}
		p.tok = _EOF
	}
}

// advance skips tokens until it finds a synchronization point.
// This is used for error recovery.
func (p *Parser) advance() {
	sync := map[Token]bool{
		_Semi:     true, // statement terminator
		_Rbrace:   true, // block end
		_Rparen:   true, // param list end
		_Rbrack:   true, // index end
		_Package:  true,
		_Import:   true,
		_Type:     true,
		_Var:      true,
		_Func:     true,
		_If:       true,
		_For:      true,
		_Return:   true,
		_Break:    true,
		_Continue: true,
		_EOF:      true,
	}

	for p.tok != _EOF && !sync[p.tok] {
		p.next()
	}

	// Consume sync point to avoid repeated errors at the same position
	if p.tok != _EOF {
		p.next()
	}
}

// Errors returns the number of errors encountered during parsing.
func (p *Parser) Errors() int {
	return p.errcnt
}

// FirstError returns the first error encountered, or nil if none.
func (p *Parser) FirstError() error {
	return p.first
}

// ----------------------------------------------------------------------------
// Parsing entry point

// Parse parses a complete source file and returns the AST.
func (p *Parser) Parse() *File {
	f := &File{}
	f.pos = p.pos

	// Parse package declaration
	p.want(_Package)
	f.PkgName = p.name()
	p.want(_Semi)

	// Parse import declarations
	for !p.abort && p.tok == _Import {
		f.Imports = append(f.Imports, p.importDecl())
	}

	// Parse top-level declarations
	for !p.abort && p.tok != _EOF {
		// Skip any semicolons between declarations (ASI inserts them after })
		for p.tok == _Semi {
			p.next()
		}
		if p.tok == _EOF {
			break
		}
		if d := p.decl(); d != nil {
			f.Decls = append(f.Decls, d)
		}
	}

	return f
}

// ----------------------------------------------------------------------------
// Helper methods

// name parses an identifier and returns a Name node.
func (p *Parser) name() *Name {
	if p.tok != _Name {
		p.syntaxError("expected identifier")
		// Return a placeholder for error recovery
		n := &Name{Value: "_"}
		n.pos = p.pos
		return n
	}
	n := &Name{Value: p.lit}
	n.pos = p.pos
	p.next()
	return n
}

// ----------------------------------------------------------------------------
// Import declarations

// importDecl parses: import "path"
func (p *Parser) importDecl() *ImportDecl {
	d := &ImportDecl{}
	d.pos = p.pos

	p.want(_Import)

	if p.tok != _Literal || p.scanner.LitKind() != StringLit {
		p.syntaxError("expected string literal for import path")
		p.advance()
		return d
	}

	d.Path = &BasicLit{Value: p.lit, Kind: StringLit}
	d.Path.pos = p.pos
	p.next()

	p.want(_Semi)
	return d
}

// ----------------------------------------------------------------------------
// Top-level declarations

// decl parses a top-level declaration.
func (p *Parser) decl() Decl {
	switch p.tok {
	case _Type:
		return p.typeDecl()
	case _Var:
		return p.varDecl()
	case _Func:
		return p.funcDecl()
	default:
		p.syntaxError("expected declaration")
		p.advance()
		return nil
	}
}

// ----------------------------------------------------------------------------
// Type declarations

// typeDecl parses: type Name Type or type Name = Type
func (p *Parser) typeDecl() *TypeDecl {
	d := &TypeDecl{}
	d.pos = p.pos

	p.want(_Type)
	d.Name = p.name()

	// Check for alias (=)
	if p.got(_Assign) {
		d.Alias = true
	}

	d.Type = p.type_()
	p.want(_Semi)

	return d
}

// type_ parses a type expression.
func (p *Parser) type_() Expr {
	switch p.tok {
	case _Name:
		return p.typeName()

	case _Mul: // *T
		return p.pointerType()

	case _Ref: // ref T
		return p.refType()

	case _Lbrack: // [N]T
		return p.arrayType()

	case _Struct:
		return p.structType()

	default:
		p.syntaxError("expected type")
		n := &Name{Value: "_"}
		n.pos = p.pos
		return n
	}
}

// typeName parses a type name (identifier).
func (p *Parser) typeName() Expr {
	return p.name()
}

// pointerType parses *Base
func (p *Parser) pointerType() Expr {
	pt := &PointerType{}
	pt.pos = p.pos
	p.want(_Mul)
	pt.Base = p.type_()
	return pt
}

// refType parses ref Base
func (p *Parser) refType() Expr {
	rt := &RefType{}
	rt.pos = p.pos
	p.want(_Ref)
	rt.Base = p.type_()
	return rt
}

// arrayType parses [N]Elem
func (p *Parser) arrayType() Expr {
	at := &ArrayType{}
	at.pos = p.pos
	p.want(_Lbrack)
	at.Len = p.expr()
	p.want(_Rbrack)
	at.Elem = p.type_()
	return at
}

// structType parses struct { Fields... }
func (p *Parser) structType() Expr {
	st := &StructType{}
	st.pos = p.pos

	p.want(_Struct)
	p.want(_Lbrace)

	for p.tok != _Rbrace && p.tok != _EOF {
		st.Fields = append(st.Fields, p.fieldDecl())
	}

	p.want(_Rbrace)
	return st
}

// fieldDecl parses a struct field: Name Type
func (p *Parser) fieldDecl() *Field {
	f := &Field{}
	f.pos = p.pos
	f.Name = p.name()
	f.Type = p.type_()
	p.want(_Semi) // ASI handles newline
	return f
}

// ----------------------------------------------------------------------------
// Variable declarations

// varDecl parses: var Name Type = Value
func (p *Parser) varDecl() *VarDecl {
	d := &VarDecl{}
	d.pos = p.pos

	p.want(_Var)
	d.Name = p.name()

	// Type is optional if there's an initializer
	if p.tok != _Assign {
		d.Type = p.type_()
	}

	// Optional initializer
	if p.got(_Assign) {
		d.Value = p.expr()
	}

	p.want(_Semi)
	return d
}

// ----------------------------------------------------------------------------
// Function declarations

// funcDecl parses: func (recv) Name(params) result { body }
func (p *Parser) funcDecl() *FuncDecl {
	d := &FuncDecl{}
	d.pos = p.pos

	p.want(_Func)

	// Optional receiver
	if p.tok == _Lparen {
		d.Recv = p.receiver()
	}

	d.Name = p.name()
	d.Params = p.paramList()

	// Optional result type
	if p.tok != _Lbrace {
		d.Result = p.type_()
	}

	p.fnest++
	d.Body = p.blockStmt()
	p.fnest--

	return d
}

// receiver parses (name Type)
func (p *Parser) receiver() *Field {
	f := &Field{}
	f.pos = p.pos

	p.want(_Lparen)
	f.Name = p.name()
	f.Type = p.type_()
	p.want(_Rparen)

	return f
}

// paramList parses (p1 T1, p2 T2, ...)
func (p *Parser) paramList() []*Field {
	p.want(_Lparen)

	var params []*Field
	if p.tok != _Rparen {
		params = p.fieldList()
	}

	p.want(_Rparen)
	return params
}

// fieldList parses a comma-separated list of name type pairs.
func (p *Parser) fieldList() []*Field {
	var fields []*Field

	for {
		f := &Field{}
		f.pos = p.pos
		f.Name = p.name()
		f.Type = p.type_()
		fields = append(fields, f)

		if !p.got(_Comma) {
			break
		}
	}

	return fields
}

// ----------------------------------------------------------------------------
// Statements

// stmt parses a statement.
func (p *Parser) stmt() Stmt {
	switch p.tok {
	case _Lbrace:
		return p.blockStmt()

	case _If:
		return p.ifStmt()

	case _For:
		return p.forStmt()

	case _Return:
		return p.returnStmt()

	case _Break, _Continue:
		return p.branchStmt()

	case _Var:
		d := p.varDecl()
		s := &DeclStmt{Decl: d}
		if d != nil {
			s.pos = d.Pos()
		}
		return s

	case _Semi:
		s := &EmptyStmt{}
		s.pos = p.pos
		p.next()
		return s

	default:
		return p.simpleStmt()
	}
}

// simpleStmt parses an expression statement or assignment.
func (p *Parser) simpleStmt() Stmt {
	pos := p.pos
	x := p.expr()

	switch p.tok {
	case _Assign, _Define:
		// Assignment or short declaration
		return p.assignStmt(pos, x)

	default:
		// Expression statement
		s := &ExprStmt{X: x}
		s.pos = pos
		p.want(_Semi)
		return s
	}
}

// assignStmt parses LHS op RHS where op is = or :=
func (p *Parser) assignStmt(pos Pos, lhs Expr) Stmt {
	s := &AssignStmt{Op: p.tok, LHS: []Expr{lhs}}
	s.pos = pos

	p.next() // consume = or :=

	s.RHS = []Expr{p.expr()}
	p.want(_Semi)

	return s
}

// blockStmt parses { stmts... }
func (p *Parser) blockStmt() *BlockStmt {
	b := &BlockStmt{}
	b.pos = p.pos

	p.want(_Lbrace)

	for p.tok != _Rbrace && p.tok != _EOF {
		b.Stmts = append(b.Stmts, p.stmt())
	}

	b.Rbrace = p.pos
	p.want(_Rbrace)
	// Note: ASI handles semicolon after }

	return b
}

// ifStmt parses: if cond { then } [else { else }]
func (p *Parser) ifStmt() Stmt {
	s := &IfStmt{}
	s.pos = p.pos

	p.want(_If)
	s.Cond = p.expr()
	s.Then = p.blockStmt()

	if p.got(_Else) {
		if p.tok == _If {
			s.Else = p.ifStmt() // else if
		} else {
			s.Else = p.blockStmt() // else
		}
	}

	return s
}

// forStmt parses: for cond { body }
func (p *Parser) forStmt() Stmt {
	s := &ForStmt{}
	s.pos = p.pos

	p.want(_For)

	// Yoru only supports for cond { ... }; bare "for { ... }" is rejected.
	if p.tok == _Lbrace {
		p.syntaxError("expected for condition")
	} else {
		s.Cond = p.expr()
	}

	s.Body = p.blockStmt()
	return s
}

// returnStmt parses: return [expr]
func (p *Parser) returnStmt() Stmt {
	s := &ReturnStmt{}
	s.pos = p.pos

	p.want(_Return)

	// Optional return value (check for statement terminators)
	if p.tok != _Semi && p.tok != _Rbrace && p.tok != _EOF {
		s.Result = p.expr()
	}

	p.want(_Semi)
	return s
}

// branchStmt parses: break or continue
func (p *Parser) branchStmt() Stmt {
	s := &BranchStmt{Tok: p.tok}
	s.pos = p.pos
	p.next()
	p.want(_Semi)
	return s
}

// ----------------------------------------------------------------------------
// Expressions

// expr parses an expression.
func (p *Parser) expr() Expr {
	return p.binaryExpr(0)
}

// binaryExpr parses a binary expression with minimum precedence prec.
// Implements Pratt parsing / precedence climbing.
func (p *Parser) binaryExpr(prec int) Expr {
	x := p.unaryExpr()

	for {
		// Check if current token is a binary operator with sufficient precedence
		oprec := p.tok.Precedence()
		if oprec <= prec {
			return x
		}

		// Binary expression position starts at the left operand.
		op := &Operation{Op: p.tok, X: x}
		op.pos = x.Pos()

		p.next() // consume operator

		// Parse right operand with higher precedence (left associative)
		op.Y = p.binaryExpr(oprec)
		x = op
	}
}

// unaryExpr parses a unary expression.
func (p *Parser) unaryExpr() Expr {
	switch p.tok {
	case _Not: // !
		op := &Operation{Op: p.tok}
		op.pos = p.pos
		p.next()
		op.X = p.unaryExpr()
		return op

	case _Sub: // - (negation)
		op := &Operation{Op: p.tok}
		op.pos = p.pos
		p.next()
		op.X = p.unaryExpr()
		return op

	case _Mul: // * (dereference)
		op := &Operation{Op: p.tok}
		op.pos = p.pos
		p.next()
		op.X = p.unaryExpr()
		return op

	case _And: // & (address-of)
		op := &Operation{Op: p.tok}
		op.pos = p.pos
		p.next()
		op.X = p.unaryExpr()
		return op

	default:
		return p.primaryExpr()
	}
}

// primaryExpr parses primary expressions and postfix operations.
func (p *Parser) primaryExpr() Expr {
	x := p.operand()

	// Parse postfix operations: calls, index, selector
	for {
		switch p.tok {
		case _Lparen: // function call
			x = p.callExpr(x)

		case _Lbrack: // index expression
			x = p.indexExpr(x)

		case _Dot: // selector expression
			x = p.selectorExpr(x)

		default:
			return x
		}
	}
}

// operand parses an operand (the base of primary expressions).
func (p *Parser) operand() Expr {
	switch p.tok {
	case _Name:
		n := &Name{Value: p.lit}
		n.pos = p.pos
		p.next()
		// Check for composite literal: T{...}
		if p.tok == _Lbrace {
			return p.compositeLit(n)
		}
		return n

	case _Panic:
		// panic is lexically a keyword, but syntactically treated as a builtin function
		n := &Name{Value: "panic"}
		n.pos = p.pos
		p.next()
		return n

	case _Literal:
		lit := &BasicLit{Value: p.lit, Kind: p.scanner.LitKind()}
		lit.pos = p.pos
		p.next()
		return lit

	case _Lparen: // parenthesized expression
		pos := p.pos
		p.next()
		x := p.expr()
		p.want(_Rparen)
		paren := &ParenExpr{X: x}
		paren.pos = pos
		return paren

	case _New: // new(Type)
		return p.newExpr()

	default:
		p.syntaxError("expected operand")
		n := &Name{Value: "_"} // error recovery
		n.pos = p.pos
		return n
	}
}

// callExpr parses Fun(args...)
func (p *Parser) callExpr(fun Expr) Expr {
	call := &CallExpr{Fun: fun}
	call.pos = fun.Pos()

	p.want(_Lparen)
	if p.tok != _Rparen {
		call.Args = p.exprList()
	}
	p.want(_Rparen)

	return call
}

// indexExpr parses X[Index]
func (p *Parser) indexExpr(x Expr) Expr {
	idx := &IndexExpr{X: x}
	idx.pos = x.Pos()

	p.want(_Lbrack)
	idx.Index = p.expr()
	p.want(_Rbrack)

	return idx
}

// selectorExpr parses X.Sel
func (p *Parser) selectorExpr(x Expr) Expr {
	sel := &SelectorExpr{X: x}
	sel.pos = x.Pos()

	p.want(_Dot)
	sel.Sel = p.name()

	return sel
}

// newExpr parses new(Type)
func (p *Parser) newExpr() Expr {
	n := &NewExpr{}
	n.pos = p.pos

	p.want(_New)
	p.want(_Lparen)
	n.Type = p.type_()
	p.want(_Rparen)

	return n
}

// compositeLit parses T{elem1, key: value, ...}
func (p *Parser) compositeLit(typ Expr) Expr {
	lit := &CompositeLit{Type: typ}
	lit.pos = typ.Pos()

	p.want(_Lbrace)
	for p.tok != _Rbrace && p.tok != _EOF {
		elem := p.expr()
		if p.got(_Colon) {
			kv := &KeyValueExpr{Key: elem}
			kv.pos = elem.Pos()
			kv.Value = p.expr()
			lit.Elems = append(lit.Elems, kv)
		} else {
			lit.Elems = append(lit.Elems, elem)
		}
		if !p.got(_Comma) {
			break
		}
	}
	p.want(_Rbrace)
	return lit
}

// exprList parses a comma-separated list of expressions.
func (p *Parser) exprList() []Expr {
	list := []Expr{p.expr()}
	for p.got(_Comma) {
		list = append(list, p.expr())
	}
	return list
}
