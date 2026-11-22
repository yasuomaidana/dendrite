package export

import "strconv"

var goSyntax = syntax{
	str: strconv.Quote, num: decimal, addr: hexAddr(""), flag: boolLit("true", "false"),
	list: listFn(func(elem string) string { return "[]" + elem + "{" }, "}"),
	rec:  recFn(func(typ string) string { return typ + "{" }, func(n string) string { return camel(n) + ": " }, "}"),
}

var goLang = language{
	ext: "go", sx: goSyntax, tab: "\t",
	header:   "// Structure of %s (analysis %s), written by `dendrite retro export`. Data only: no code, no dependencies.\npackage analysis\n\n",
	typeOpen: "type %s struct {\n", typeField: "\t%s %s\n", close: "}\n\n", fieldName: camel,
	tStr: "string", tNum: "uint64", tAddr: "uint64", tFlag: "bool",
	tList: func(elem string) string { return "[]" + elem },
	tail:  "var Analysis = %s\n",
}
