package types

// Package represents a Yoru package.
type Package struct {
	name  string // package name (e.g., "main")
	path  string // import path (for future use)
	scope *Scope // package-level scope
}

// NewPackage creates a new package with the given name.
func NewPackage(name string) *Package {
	return &Package{
		name:  name,
		scope: NewScope(Universe, NoPos, NoPos, "package "+name),
	}
}

// Name returns the package name.
func (p *Package) Name() string {
	return p.name
}

// Path returns the package import path.
func (p *Package) Path() string {
	return p.path
}

// SetPath sets the package import path.
func (p *Package) SetPath(path string) {
	p.path = path
}

// Scope returns the package-level scope.
func (p *Package) Scope() *Scope {
	return p.scope
}

// String returns the package name.
func (p *Package) String() string {
	return p.name
}
