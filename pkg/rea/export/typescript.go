package export

var tsLang = language{
	ext: "ts", suffix: "n", yes: "true", no: "false", str: jsonString, listOpen: "[", listClose: "]", recOpen: "{", field: "$: ", recClose: "}", tab: "  ",
	header:   "// Structure of %s (analysis %s), written by `dendrite retro export`. Data only: no code, no dependencies.\n\n",
	typeOpen: "export interface %s {\n", typeField: "\treadonly %s: %s;\n", close: "}\n\n",
	tStr: "string", tNum: "number", tAddr: "bigint", tFlag: "boolean",
	tList: func(elem string) string { return "readonly " + elem + "[]" },
	tail:  "export const IMAGE: Image = %s;\n",
}
