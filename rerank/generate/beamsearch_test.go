package generate

import (
	"context"
	"fmt"
	"log"
	"testing"
)

func TestGenCansBasic(t *testing.T) {
	bs := &BeamSearcher{
		seqCount:  5,
		seqLength: 10,
		beamWidth: 3,
		win:       &Window{Size: 5, Limit: 2},
	}

	tags := map[string]*TagData{
		"tag1": {
			Units: []*Unit{
				{ID: "a1", Score: 3.0},
				{ID: "a2", Score: 2.5},
				{ID: "a3", Score: 3.0},
				{ID: "a4", Score: 2.5},
				{ID: "a5", Score: 3.0},
				{ID: "a6", Score: 2.5},
				{ID: "a7", Score: 3.0},
				{ID: "a8", Score: 2.5},
				{ID: "a9", Score: 2.8},
				{ID: "a10", Score: 3.2},
			},
		},
		"tag2": {
			Units: []*Unit{
				{ID: "b1", Score: 2.0},
				{ID: "b2", Score: 2.1},
				{ID: "b3", Score: 1.8},
				{ID: "b4", Score: 2.3},
				{ID: "b5", Score: 2.0},
			},
		},
		"tag3": {
			Units: []*Unit{
				{ID: "c1", Score: 1.5},
				{ID: "c2", Score: 1.7},
				{ID: "c3", Score: 1.9},
				{ID: "c4", Score: 2.0},
				{ID: "c5", Score: 1.8},
				{ID: "c6", Score: 1.6},
			},
		},
		"tag4": {
			Units: []*Unit{
				{ID: "d1", Score: 3.5},
				{ID: "d2", Score: 3.6},
			},
		},
	}
	beams, err := bs.Generate(context.Background(), tags)
	if err != nil {
		log.Fatal(err)
	}
	for i, res := range beams {
		fmt.Printf("Result %d (Score: %.2f):\n", i+1, res.Score)
		for _, unit := range res.Units {
			fmt.Printf("  %s (%s): %.2f\n", unit.ID, unit.Tag, unit.Score)
		}
	}
}
