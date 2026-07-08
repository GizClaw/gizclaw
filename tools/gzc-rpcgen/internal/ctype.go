package rpcgen

type CTypeKind int

const (
	CTypeString CTypeKind = iota
	CTypeBool
	CTypeI32
	CTypeI64
	CTypeF64
	CTypeJSON
)

type CType struct {
	Kind CTypeKind
	Name string
}
