package export

var pySyntax = syntax{
	str: jsonString, suffix: "", yes: "True", no: "False",
	list: listFn(fixed("("), ")"),
	rec:  recFn(func(typ string) string { return typ + "(" }, func(n string) string { return n + "=" }, ")"),
}

var pyLang = language{
	ext: "py", sx: pySyntax, tab: "    ",
	header:   "\"\"\"Structure of %s (analysis %s), written by `dendrite retro export`. Data only: no code, no dependencies.\"\"\"\n\nfrom __future__ import annotations\n\nfrom dataclasses import dataclass\n\n\n",
	typeOpen: "@dataclass(frozen=True)\nclass %s:\n", typeField: "\t%s: %s\n", close: "\n\n",
	tStr: "str", tNum: "int", tAddr: "int", tFlag: "bool",
	tList: func(elem string) string { return "tuple[" + elem + ", ...]" },
	tail:  "IMAGE = %s\n",
}
