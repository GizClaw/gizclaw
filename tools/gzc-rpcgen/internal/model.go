package rpcgen

type Config struct {
	SchemaPath  string
	IncludeDirs []string
	OutDir      string
	Package     string
	Check       bool
	Format      bool
}

type Model struct {
	Package string
	Methods []Method
}

type Method struct {
	Index        int
	Value        string
	ConstName    string
	RequestName  string
	ResponseName string
	Kind         string
	Request      Schema
	Response     Schema
}

type Schema struct {
	Name     string
	Fields   []Field
	Original map[string]any
}

type Field struct {
	JSONName string
	CName    string
	Type     CType
	Required bool
}
