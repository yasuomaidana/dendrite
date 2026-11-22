package export

var tsSyntax = syntax{
	str: jsonString, num: decimal, addr: hexAddr("n"), flag: boolLit("true", "false"),
	list: listFn(fixed("["), "]"),
	rec:  recFn(fixed("{"), func(n string) string { return n + ": " }, "}"),
}

var tsLang = language{
	ext: "ts", sx: tsSyntax, tab: "  ",
	header:   "// Structure of %s (analysis %s), written by `dendrite retro export`. Data only: no code, no dependencies.\n\n",
	typeOpen: "export interface %s {\n", typeField: "\treadonly %s: %s;\n", close: "}\n\n",
	tStr: "string", tNum: "number", tAddr: "bigint", tFlag: "boolean",
	tList: func(elem string) string { return "readonly " + elem + "[]" },
	tail:  "export const IMAGE: Image = %s;\n",
}
