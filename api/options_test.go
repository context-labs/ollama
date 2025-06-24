package api

import "testing"

func TestValidateLogProbs(t *testing.T) {
    cases := []struct {
        name    string
        opts    Options
        wantErr bool
        wantTop int
    }{
        {"disabled", Options{}, false, 0},
        {"enabled default", Options{LogProbsEnabled: true}, false, 1},
        {"enabled 3", Options{LogProbsEnabled: true, TopLogProbs: 3}, false, 3},
        {"enabled max", Options{LogProbsEnabled: true, TopLogProbs: 5}, false, 5},
        {"enabled too high", Options{LogProbsEnabled: true, TopLogProbs: 6}, true, 6},
        {"enabled negative", Options{LogProbsEnabled: true, TopLogProbs: -1}, true, -1},
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            err := tc.opts.ValidateLogProbs()
            if tc.wantErr {
                if err == nil {
                    t.Fatalf("expected error, got nil")
                }
            } else if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }

            if !tc.wantErr && tc.opts.LogProbsEnabled {
                if tc.opts.TopLogProbs != tc.wantTop {
                    t.Fatalf("expected TopLogProbs %d, got %d", tc.wantTop, tc.opts.TopLogProbs)
                }
            }
        })
    }
}