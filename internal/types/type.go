// Package types implements the type system for the Yoru programming language.
// This package provides type representations without AST dependencies.
package types

// Type is the interface implemented by all types.
type Type interface {
	// Underlying returns the underlying type.
	// For Named types, returns the type it names.
	// For all other types, returns the receiver.
	Underlying() Type

	// String returns a human-readable representation of the type.
	String() string

	// aType is a marker method to restrict implementations to this package.
	aType()
}

// typ is a base struct for all type implementations.
type typ struct{}

func (typ) aType() {}
