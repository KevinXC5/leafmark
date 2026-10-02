import { Facet } from "@codemirror/state";

/** 共享实际整块替换范围，让行内预览跳过 widget；未安装扩展时为空。 */
export const liveBlockRanges = Facet.define<readonly { from: number; to: number }[], readonly { from: number; to: number }[]>({
  combine: values => values.flat(),
});
