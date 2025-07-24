package generate

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/ongniud/other/rerank/generate/common"
)

type Unit struct {
	ID    string
	Tag   string
	Score float64
}

type TagData struct {
	Units []*Unit
}

type Candidate struct {
	Units  []*Unit
	Score  float64
	Refs   map[string]int
	Counts map[string]int
	IDs    map[string]struct{}

	Win *common.CounterWindow
}

func (c *Candidate) Clone() *Candidate {
	refs := make(map[string]int, len(c.Refs))
	for tag, ref := range c.Refs {
		refs[tag] = ref
	}
	counts := make(map[string]int, len(c.Refs))
	for tag, cnt := range c.Counts {
		counts[tag] = cnt
	}
	units := make([]*Unit, len(c.Units))
	copy(units, c.Units)

	ids := make(map[string]struct{}, len(c.IDs))
	for id := range c.IDs {
		ids[id] = struct{}{}
	}

	return &Candidate{
		Units:  units,
		Refs:   refs,
		Counts: counts,
		Score:  c.Score,
		IDs:    ids,
		Win:    c.Win.Clone(),
	}
}

type Window struct {
	Size  int
	Limit int
}

type BeamSearcher struct {
	seqCount  int
	seqLength int
	beamWidth int

	maxPerTag map[string]int // 每个 tag 最大使用次数

	win *Window
}

func (s *BeamSearcher) Generate(ctx context.Context, tags map[string]*TagData) ([]*Candidate, error) {
	initial := &Candidate{
		Units:  []*Unit{},
		Score:  0,
		Refs:   make(map[string]int),
		Counts: make(map[string]int),
		IDs:    make(map[string]struct{}),
	}

	if s.win != nil {
		win, err := common.NewCounterWindow(s.win.Size, s.win.Limit)
		if err != nil {
			return nil, err
		}
		initial.Win = win
	}

	candidates := []*Candidate{initial}
	for i := 0; i < s.seqLength; i++ {
		var beams []*Candidate
		for _, can := range candidates {
			newCans := s.genCans(can, tags)
			if newCans != nil {
				beams = append(beams, newCans...)
				continue
			}
			return nil, fmt.Errorf("%s", "no candidates")
		}

		if len(beams) > s.beamWidth {
			sort.Slice(beams, func(i, j int) bool {
				return beams[i].Score > beams[j].Score
			})
			candidates = beams[:s.beamWidth]
		} else {
			candidates = beams
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	if len(candidates) > s.seqCount {
		candidates = candidates[:s.seqCount]
	}

	return candidates, nil
}

func (s *BeamSearcher) genCans(can *Candidate, tags map[string]*TagData) []*Candidate {
	var beams []*Candidate
	for tagKey, tagData := range tags {
		count := can.Counts[tagKey]
		if s.maxPerTag[tagKey] > 0 && count >= s.maxPerTag[tagKey] {
			continue
		}

		if !can.Win.Try([]string{tagKey}) {
			continue
		}

		units := tagData.Units
		ref := can.Refs[tagKey]
		if ref >= len(units) {
			continue
		}

		for ref < len(units) {
			unit := units[ref]
			if _, ok := can.IDs[unit.ID]; ok {
				ref++
				continue
			}

			newCan := can.Clone()
			newCan.Units = append(newCan.Units, &Unit{ID: unit.ID, Tag: tagKey, Score: unit.Score})
			newCan.Refs[tagKey] = ref + 1
			newCan.Counts[tagKey] = count + 1
			newCan.IDs[unit.ID] = struct{}{}
			newCan.Score = s.calcScore(tags, newCan)
			newCan.Win.Add([]string{tagKey})
			beams = append(beams, newCan)
			break
		}
	}

	return beams
}

func (s *BeamSearcher) calcScore(tags map[string]*TagData, can *Candidate) float64 {
	seq := can.Units
	if len(seq) == 0 {
		return 0
	}

	// 1. 质量分（归一化处理）
	quality := 0.0
	for _, u := range seq {
		quality += u.Score
	}
	quality /= float64(len(seq)) // 平均质量分

	// 2. 多样性分（考虑标签分布均匀性）
	diversity := 0.0
	if len(tags) > 1 {
		tagCount := make(map[string]int)
		for _, u := range seq {
			tagCount[u.Tag]++
		}
		// 计算熵值作为多样性度量
		entropy := 0.0
		total := float64(len(seq))
		for _, count := range tagCount {
			p := float64(count) / total
			entropy -= p * math.Log(p)
		}
		diversity = entropy / math.Log(float64(len(tags)))
	}

	// 3. 连续性惩罚（指数增长）
	penalty := 0.0
	continuous := 1
	for i := 1; i < len(seq); i++ {
		if seq[i].Tag == seq[i-1].Tag {
			continuous++
			penalty += 0.1 * math.Pow(1.5, float64(continuous))
		} else {
			continuous = 1
		}
	}

	return 0.5*quality + 0.5*diversity - penalty
}
