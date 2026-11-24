// Dendrite REA: lists the functions of the program Ghidra just imported and prints them as one JSON document between
// two marker lines, because Ghidra's own log goes to the same output. For each function: name, address, size,
// cyclomatic complexity (edges - nodes + 2 over its basic blocks) and the functions it calls; for the largest ones
// also the disassembly and the decompiled C. Run by `analyzeHeadless ... -postScript ExportFunctions.java`.
import ghidra.app.decompiler.DecompInterface;
import ghidra.app.decompiler.DecompileResults;
import ghidra.app.script.GhidraScript;
import ghidra.program.model.address.AddressSetView;
import ghidra.program.model.block.BasicBlockModel;
import ghidra.program.model.block.CodeBlock;
import ghidra.program.model.block.CodeBlockIterator;
import ghidra.program.model.block.CodeBlockReference;
import ghidra.program.model.block.CodeBlockReferenceIterator;
import ghidra.program.model.listing.Function;
import ghidra.program.model.listing.Instruction;

import java.util.ArrayList;
import java.util.Comparator;
import java.util.List;
import java.util.Set;

public class ExportFunctions extends GhidraScript {
    private static final int DETAIL = 20;
    private static final int MAX_FUNCTIONS = 500;
    private static final int MAX_LINES = 80;

    @Override
    public void run() throws Exception {
        List<Function> functions = new ArrayList<>();
        for (Function f : currentProgram.getFunctionManager().getFunctions(true)) {
            functions.add(f);
        }
        functions.sort(Comparator.comparingLong((Function f) -> f.getBody().getNumAddresses()).reversed());

        DecompInterface decompiler = new DecompInterface();
        decompiler.openProgram(currentProgram);
        BasicBlockModel blocks = new BasicBlockModel(currentProgram);

        StringBuilder out = new StringBuilder("---DENDRITE-JSON-BEGIN---\n[");
        int written = 0;
        for (Function f : functions) {
            if (written == MAX_FUNCTIONS) {
                break;
            }
            if (written > 0) {
                out.append(',');
            }
            boolean detailed = written < DETAIL;
            out.append(describe(f, blocks, decompiler, detailed));
            written++;
        }
        out.append("]\n---DENDRITE-JSON-END---");
        println(out.toString());
    }

    private String describe(Function f, BasicBlockModel blocks, DecompInterface decompiler, boolean detailed) throws Exception {
        AddressSetView body = f.getBody();
        StringBuilder sb = new StringBuilder("{");
        sb.append("\"name\":").append(quote(f.getName()));
        sb.append(",\"address\":").append(quote("0x" + Long.toHexString(f.getEntryPoint().getOffset())));
        sb.append(",\"size\":").append(body.getNumAddresses());
        sb.append(",\"cyclomatic\":").append(cyclomatic(body, blocks));
        sb.append(",\"calls\":[");
        boolean first = true;
        Set<Function> called = f.getCalledFunctions(monitor);
        for (Function c : called) {
            sb.append(first ? "" : ",").append(quote("0x" + Long.toHexString(c.getEntryPoint().getOffset())));
            first = false;
        }
        sb.append("]");
        if (detailed) {
            sb.append(",\"disassembly\":").append(quote(disassembly(body)));
            DecompileResults result = decompiler.decompileFunction(f, 60, monitor);
            if (result != null && result.decompileCompleted() && result.getDecompiledFunction() != null) {
                sb.append(",\"pseudocode\":").append(quote(result.getDecompiledFunction().getC()));
            }
        }
        return sb.append("}").toString();
    }

    private int cyclomatic(AddressSetView body, BasicBlockModel blocks) throws Exception {
        int nodes = 0;
        int edges = 0;
        CodeBlockIterator it = blocks.getCodeBlocksContaining(body, monitor);
        while (it.hasNext()) {
            CodeBlock block = it.next();
            nodes++;
            CodeBlockReferenceIterator dests = block.getDestinations(monitor);
            while (dests.hasNext()) {
                CodeBlockReference ref = dests.next();
                if (body.contains(ref.getDestinationAddress())) {
                    edges++;
                }
            }
        }
        return Math.max(1, edges - nodes + 2);
    }

    private String disassembly(AddressSetView body) {
        StringBuilder sb = new StringBuilder();
        int lines = 0;
        for (Instruction insn : currentProgram.getListing().getInstructions(body, true)) {
            if (lines++ == MAX_LINES) {
                break;
            }
            sb.append("0x").append(Long.toHexString(insn.getAddress().getOffset())).append("  ").append(insn).append('\n');
        }
        return sb.toString();
    }

    private static String quote(String s) {
        StringBuilder sb = new StringBuilder("\"");
        for (char c : s.toCharArray()) {
            switch (c) {
                case '"': sb.append("\\\""); break;
                case '\\': sb.append("\\\\"); break;
                case '\n': sb.append("\\n"); break;
                case '\r': sb.append("\\r"); break;
                case '\t': sb.append("\\t"); break;
                default:
                    if (c < 0x20) {
                        sb.append(String.format("\\u%04x", (int) c));
                    } else {
                        sb.append(c);
                    }
            }
        }
        return sb.append('"').toString();
    }
}
