package main

import "testing"

func TestUseZlib(t *testing.T) {
	script := "Unicode true\r\n  SetCompressor /SOLID lzma\r\nsetcompressor bzip2\n!define SetCompressorName x\nName \"SetCompressor\"\n"
	want := "Unicode true\r\nSetCompressor zlib\r\nSetCompressor zlib\n!define SetCompressorName x\nName \"SetCompressor\"\n"
	got, count := useZlib([]byte(script))
	if string(got) != want || count != 2 {
		t.Fatalf("useZlib 替换了 %d 处，结果为 %q", count, got)
	}
	if got, count := useZlib([]byte("Name x\n")); string(got) != "Name x\n" || count != 0 {
		t.Fatalf("没有压缩器指令时不应改动脚本，得到 %q（%d 处）", got, count)
	}
}
