package common

import "errors"

type CounterWindow struct {
	size       int //>=2
	limit      int
	elems      [][]string
	elemCounts map[string]int
}

func NewCounterWindow(size, limit int) (*CounterWindow, error) {
	if size < 2 || limit < 1 || size < limit {
		return nil, errors.New("param invalid")
	}
	return &CounterWindow{
		size:       size,
		limit:      limit,
		elemCounts: make(map[string]int),
	}, nil
}

// Try 尝试插入
func (w *CounterWindow) Try(keys []string) bool {
	if w == nil || keys == nil {
		return true
	}
	return w.check(keys)
}

// Add 强制插入
func (w *CounterWindow) Add(keys []string) {
	if w == nil {
		return
	}
	if keys == nil {
		w.push(nil)
		return
	}
	w.push(keys)
}

// Adapt 自适应
func (w *CounterWindow) Adapt(keys []string) {
	if w == nil {
		return
	}
	if keys == nil {
		w.push(nil)
		return
	}
	for !w.check(keys) {
		w.pop()
	}
	w.push(keys)
}

func (w *CounterWindow) pop() {
	if len(w.elems) == 0 {
		return
	}

	out := w.elems[0]
	w.elems = w.elems[1:]
	for _, k := range out {
		if v, ok := w.elemCounts[k]; ok {
			if v == 1 {
				delete(w.elemCounts, k)
			} else {
				w.elemCounts[k]--
			}
		}
	}
}

func (w *CounterWindow) push(ks []string) {
	for _, k := range ks {
		if _, ok := w.elemCounts[k]; !ok {
			w.elemCounts[k] = 1
		} else {
			w.elemCounts[k]++
		}
	}
	w.elems = append(w.elems, ks)
	if len(w.elems) > w.size-1 {
		w.pop()
	}
}

func (w *CounterWindow) check(ks []string) bool {
	accept := true
	for _, k := range ks {
		if !w.checkThreshold(k) {
			accept = false
			break
		}
	}
	return accept
}

func (w *CounterWindow) checkThreshold(e string) bool {
	cnt, ok := w.elemCounts[e]
	if ok && cnt+1 > w.limit {
		return false
	}
	return true
}

func (w *CounterWindow) Clone() *CounterWindow {
	if w == nil {
		return nil
	}
	elems := make([][]string, len(w.elems))
	for i, arr := range w.elems {
		if arr == nil {
			elems[i] = nil
		} else {
			elems[i] = append([]string(nil), arr...)
		}
	}
	counts := make(map[string]int, len(w.elemCounts))
	for k, v := range w.elemCounts {
		counts[k] = v
	}
	return &CounterWindow{
		size:       w.size,
		limit:      w.limit,
		elems:      elems,
		elemCounts: counts,
	}
}
