import { describe, expect, test } from "bun:test";
import { parseTable, serializeTable, splitTableRow } from "../../../src/editor/dialogs/table-editor";

describe("GFM 表格解析和序列化", () => {
  test("解析无边框表格及四种对齐", () => {
    const table = parseTable("A | B | C | D\n--- | :--- | :---: | ---:\n1 | 2 | 3 | 4");
    expect(table).toEqual({ headers: ["A", "B", "C", "D"], rows: [["1", "2", "3", "4"]], alignments: [null, "left", "center", "right"] });
    expect(parseTable(serializeTable(table!))).toEqual(table);
  });
  test("已有转义管道和 inline code 不重复转义", () => {
    const source = "| 文本 | 代码 |\n| --- | --- |\n| a\\|b | ``a\\|`b`` |";
    const table = parseTable(source)!;
    expect(table.rows).toEqual([["a\\|b", "``a\\|`b``"]]);
    expect(serializeTable(table)).toBe(source);
  });
  test("新输入的管道包括 inline code 内的管道均转义", () => {
    const table = { headers: ["A", "B"], rows: [["a|b", "`a|b`"]], alignments: [null, null] };
    const output = serializeTable(table);
    expect(output).toContain("a\\|b | `a\\|b`");
    expect(parseTable(output)?.rows).toEqual([["a\\|b", "`a\\|b`"]]);
  });
  test("反斜杠奇偶决定管道是否分列", () => {
    expect(splitTableRow("| a\\|b | c |" )).toEqual(["a\\|b", "c"]);
    expect(splitTableRow("| a\\\\| b |" )).toEqual(["a\\\\", "b"]);
    expect(splitTableRow("| a\\\\\\|b | c |" )).toEqual(["a\\\\\\|b", "c"]);
  });
  test("空单元格、缺失列和多余列遵循 GFM", () => {
    expect(parseTable("| A | B |\n| --- | --- |\n| | |\n| x |\n| x | y | z |")?.rows).toEqual([["", ""], ["x", ""], ["x", "y"]]);
    expect(parseTable("| |\n| --- |")?.headers).toEqual([""]);
  });
  test("拒绝非表格、错误表头和代码块", () => {
    for (const source of ["文字", "| A | B |\n| --- |", "| A |\n| nope |", "```\n| A |\n| --- |\n```", "| A |\n| --- |\n\n# 标题"]) expect(parseTable(source)).toBeNull();
  });
  test("支持 CRLF，换行内容转成 br，末尾反斜杠安全", () => {
    expect(parseTable("| A |\r\n| --- |\r\n| x |\r\n")?.rows).toEqual([["x"]]);
    const output = serializeTable({ headers: ["A"], rows: [["a\nb\\"]], alignments: [null] });
    expect(output).toContain("a<br>b\\\\");
    expect(parseTable(output)?.rows).toEqual([["a<br>b\\\\"]]);
  });
  test("禁止零列表格", () => {
    expect(() => serializeTable({ headers: [], rows: [], alignments: [] })).toThrow();
  });
});
