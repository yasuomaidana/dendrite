package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// signature maps the first bytes of a file to the scripted answer for that format.
type signature struct {
	prefix []byte
	image  func() map[string]interface{}
}

var signatures = []signature{
	{[]byte("MZ"), peImage},
	{[]byte("\x7fELF"), elfImage},
	{[]byte{0xcf, 0xfa, 0xed, 0xfe}, machoImage},
	{[]byte{0xca, 0xfe, 0xba, 0xbe}, fatMachoImage},
	{[]byte("PK\x03\x04"), zipImage},
}

// binInspect answers bin.inspect: a directory is a source tree, a file is recognised by its first bytes or its
// extension, and anything else is an unknown file (not an error). Mode "noinspect" plays a dendrite-core that
// predates the method.
func binInspect(params json.RawMessage, o options) (any, map[string]interface{}) {
	if o.mode == "noinspect" {
		return nil, rpcError(-32601, "method not found: bin.inspect")
	}
	var p struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(params, &p)
	info, err := os.Stat(p.Path)
	if err != nil {
		return nil, rpcError(-32602, "cannot read "+p.Path)
	}
	if info.IsDir() {
		return sourceTree(p.Path), nil
	}
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return nil, rpcError(-32602, "cannot read "+p.Path)
	}
	return byContent(p.Path, data), nil
}

func byContent(path string, data []byte) map[string]interface{} {
	for _, s := range signatures {
		if strings.HasPrefix(string(data), string(s.prefix)) {
			return s.image()
		}
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".asar":
		return asarImage()
	case ".js":
		return jsBundleImage()
	}
	return map[string]interface{}{"kind": "data", "format": "unknown", "entropy": 4.2, "strings": []string{"hello", "config"}}
}

func peImage() map[string]interface{} {
	return map[string]interface{}{"kind": "executable", "format": "pe", "arch": "x86_64", "bits": 64, "endian": "little",
		"entry": "0x140001000", "base": "0x140000000", "notes": map[string]string{"subsystem": "console"},
		"sections":  []map[string]interface{}{{"name": ".text", "address": "0x1000", "size": 4096, "raw_size": 4096, "flags": []string{"code", "execute", "read"}}},
		"libraries": []map[string]interface{}{{"name": "KERNEL32.dll", "symbols": []string{"CreateFileW", "ReadFile"}}},
		"exports":   []string{"ComputeChecksum"}}
}

func elfImage() map[string]interface{} {
	return map[string]interface{}{"kind": "executable", "format": "elf", "arch": "x86_64", "bits": 64, "endian": "little",
		"entry": "0x401040", "notes": map[string]string{"interpreter": "/lib64/ld-linux-x86-64.so.2", "rpath": "$ORIGIN"},
		"sections":  []map[string]interface{}{{"name": ".text", "address": "0x401040", "size": 2048, "flags": []string{"alloc", "execute"}}},
		"libraries": []map[string]interface{}{{"name": "libc.so.6", "symbols": []string{"printf", "strcmp"}}},
		"exports":   []string{"check_flag"}}
}

func machoImage() map[string]interface{} {
	return map[string]interface{}{"kind": "executable", "format": "macho", "arch": "arm64", "bits": 64, "endian": "little",
		"entry": "0x100003f00", "notes": map[string]string{"codesign": "adhoc"},
		"sections":  []map[string]interface{}{{"name": "__TEXT.__text", "address": "0x100003f00", "size": 1024, "flags": []string{"code"}}},
		"libraries": []map[string]interface{}{{"name": "/usr/lib/libSystem.B.dylib", "symbols": []string{"_printf"}}}}
}

func fatMachoImage() map[string]interface{} {
	img := machoImage()
	img["format"] = "fat-macho"
	img["notes"] = map[string]string{"architectures": "x86_64,arm64"}
	return img
}

func zipImage() map[string]interface{} {
	return map[string]interface{}{"kind": "package", "format": "zip",
		"files": []map[string]interface{}{{"path": "classes.dex", "size": 4096, "kind": "bytecode"}, {"path": "AndroidManifest.xml", "size": 512, "kind": "manifest"}}}
}

func asarImage() map[string]interface{} {
	return map[string]interface{}{"kind": "package", "format": "asar",
		"files": []map[string]interface{}{{"path": "main.js", "size": 2048, "kind": "script"}}, "notes": map[string]string{"integrity": "none"}}
}

func jsBundleImage() map[string]interface{} {
	return map[string]interface{}{"kind": "bundle", "format": "js-bundle", "libraries": []map[string]interface{}{{"name": "lodash"}},
		"strings": []string{"https://api.internal.endpoint"}}
}

// sourceTree lists the files directly under dir, sorted, as a source tree.
func sourceTree(dir string) map[string]interface{} {
	entries, _ := os.ReadDir(dir)
	var files []map[string]interface{}
	for _, e := range entries {
		if info, err := e.Info(); err == nil && !e.IsDir() {
			files = append(files, map[string]interface{}{"path": e.Name(), "size": info.Size(), "kind": "source"})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i]["path"].(string) < files[j]["path"].(string) })
	return map[string]interface{}{"kind": "source", "format": "source-tree", "files": files}
}
