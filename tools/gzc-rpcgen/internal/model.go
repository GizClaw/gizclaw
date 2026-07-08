package rpcgen

type Config struct {
	ProtoPath        string
	PayloadProtoPath string
	OutDir           string
	Package          string
	Check            bool
	Format           bool
}

type Model struct {
	Package string
	Methods []Method
}

type Method struct {
	Index        int
	ID           int
	Value        string
	ConstName    string
	RequestName  string
	ResponseName string
	Kind         string
	Request      Schema
	Response     Schema
}

type Schema struct {
	Name   string
	Fields []Field
}

type Field struct {
	JSONName string
	CName    string
	Number   int
	Type     CType
	Required bool
}
