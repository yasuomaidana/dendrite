package export

var tsLang = language{
	tail:     "export const IMAGE: Image = %s;\n",
	ext:      "ts",
	tab:      "  ",
	typeOpen: "export interface %s {\n", typeField: "\treadonly %s: %s;\n", close: "}\n\n",
	header: "// Structure of %s (analysis %s), written by `dendrite retro export`. Data only: no code, no dependencies.\n\n",
	tList:  "readonly $[]", tStr: "string", tNum: "number", tAddr: "bigint", tFlag: "boolean",
	str: jsonString, suffix: "n", yes: "true", no: "false",
	listOpen: "[", listClose: "]", recOpen: "{", field: "$: ", recClose: "}",
}
