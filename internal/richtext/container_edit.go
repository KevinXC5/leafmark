package richtext

// nextContainerID 只在当前文档中分配新标识，撤销快照保留已有标识。
func (d *Document) nextContainerID() int {
	n := 0
	for _, b := range d.blocks {
		for _, c := range b.Containers {
			n = max(n, c.ID)
		}
	}
	return n + 1
}

func listContext(b editBlock) int {
	for i := len(b.Containers) - 1; i >= 0; i-- {
		if b.Containers[i].Kind == List || b.Containers[i].Kind == Task {
			return i
		}
	}
	return -1
}

// refreshContainerBlock 同步叶块的展示属性，不让任务状态与祖先结构分离。
func refreshContainerBlock(b *editBlock) {
	b.Footnote = nil
	for _, c := range b.Containers {
		if c.Footnote != "" {
			b.Footnote = &FootnoteData{Label: c.Footnote}
		}
	}
	if b.Kind == List || b.Kind == Task {
		ci := listContext(*b)
		if ci < 0 {
			b.Kind, b.Level = Paragraph, 0
			return
		}
		c := b.Containers[ci]
		b.Kind, b.Level, b.Ordered, b.Start, b.Checked = c.Kind, ci+1, c.Ordered, c.Start, c.Checked
	}
}

func (d *Document) dirtyGroup(i int) {
	group := d.blocks[i].group
	if group == 0 {
		d.blocks[i].dirty = true
		return
	}
	for j := range d.blocks {
		if d.blocks[j].group == group {
			d.blocks[j].dirty = true
		}
	}
}

// leaveContainer 退出最内层，后续同项内容留在原容器内。
func (d *Document) leaveContainer(i int) {
	b := &d.blocks[i]
	b.Containers = append([]Container(nil), b.Containers[:len(b.Containers)-1]...)
	refreshContainerBlock(b)
	d.dirtyGroup(i)
}

func (d *Document) shiftContainerItem(i, delta int) bool {
	ci := listContext(d.blocks[i])
	if ci < 0 {
		return false
	}
	ctx := d.blocks[i].Containers[ci]
	end := i + 1
	for end < len(d.blocks) && len(d.blocks[end].Containers) > ci && d.blocks[end].Containers[ci].ID == ctx.ID {
		end++
	}
	chain := append([]Container(nil), d.blocks[i].Containers[:ci]...)
	if delta > 0 {
		if i == 0 {
			return false
		}
		prev := d.blocks[i-1]
		pi := listContext(prev)
		if pi < 0 {
			return false
		}
		chain = append([]Container(nil), prev.Containers[:pi+1]...)
		if len(chain) > 6 {
			return false
		}
	} else {
		if len(chain) == 0 || (chain[len(chain)-1].Kind != List && chain[len(chain)-1].Kind != Task) {
			return false
		}
		chain = chain[:len(chain)-1]
	}
	chain = append(chain, ctx)
	for j := i; j < end; j++ {
		tail := append([]Container(nil), d.blocks[j].Containers[ci+1:]...)
		d.blocks[j].Containers = append(append([]Container(nil), chain...), tail...)
		for k := range d.blocks[j].Containers {
			d.blocks[j].Containers[k].Level = k + 1
		}
		refreshContainerBlock(&d.blocks[j])
		d.blocks[j].dirty = true
	}
	d.dirtyGroup(i)
	return true
}
