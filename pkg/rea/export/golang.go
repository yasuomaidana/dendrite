package export

import "strconv"

var goLang = language{
	ext: "go", suffix: "", yes: "true", no: "false", str: strconv.Quote, listOpen: "[]${", listClose: "}", recOpen: "${", field: "$: ", recClose: "}", tab: "\t",
	header:   "// Structure of %s (analysis %s), written by `dendrite retro export`. Data only: no code, no dependencies.\npackage analysis\n\n",
	typeOpen: "type %s struct {\n", typeField: "\t%s %s\n", close: "}\n\n", fieldName: camel,
	tStr: "string", tNum: "uint64", tAddr: "uint64", tFlag: "bool",
	tList: "[]$",
	tail:  "var Analysis = %s\n",
}
